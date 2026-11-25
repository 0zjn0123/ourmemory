package om

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"gorm.io/gorm"
)

const (
	packMagic   = "OMPK1\n"
	recordMagic = "OMB1"
	recordHdr   = 4 + 1 + 12 + 8 + 8 + 32
	chunkSize   = 1 << 20
)

type PackStore struct {
	cfg   Config
	mu    sync.Mutex
	rw    sync.RWMutex
	locks map[string]*sync.Mutex
}

var Store *PackStore

func OpenStore(cfg Config) (*PackStore, error) {
	return &PackStore{cfg: cfg, locks: map[string]*sync.Mutex{}}, nil
}

func (s *PackStore) lockFor(path string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.locks[path]; ok {
		return l
	}
	l := &sync.Mutex{}
	s.locks[path] = l
	return l
}

func (s *PackStore) packPath(scope string, ownerID uint) string {
	if scope == "private" {
		return filepath.Join(s.cfg.PacksDir, fmt.Sprintf("private-%d.ompk", ownerID))
	}
	return filepath.Join(s.cfg.PacksDir, "shared.ompk")
}

func (s *PackStore) packID(scope string, ownerID uint) string {
	if scope == "private" {
		return fmt.Sprintf("private-%d", ownerID)
	}
	return "shared"
}

func (s *PackStore) ensurePack(scope string, ownerID uint) (string, error) {
	path := s.packPath(scope, ownerID)
	_, err := os.Stat(path)
	if err == nil {
		return path, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(packMagic); err != nil {
		return "", err
	}
	now := time.Now()
	pid := s.packID(scope, ownerID)
	_ = DB.FirstOrCreate(&Pack{}, Pack{ID: pid, Scope: scope, OwnerID: ownerID, Path: path, CreatedAt: now, UpdatedAt: now}).Error
	return path, nil
}

func dedupKey(scope string, ownerID uint, key []byte, sum string) string {
	if scope == "private" {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(sum))
		return "p:" + hex.EncodeToString(m.Sum(nil))
	}
	return "s:" + sum
}

func shaFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Write stores one plaintext file into the scope's archive. The caller must
// remove plainPath afterwards. key is only used for private scopes.
func (s *PackStore) Write(scope string, ownerID uint, key []byte, plainPath string, level int) (*Blob, error) {
	sum, size, err := shaFile(plainPath)
	if err != nil {
		return nil, err
	}
	if level < 1 {
		level = 7
	}
	if level > 9 {
		level = 9
	}
	dk := dedupKey(scope, ownerID, key, sum)
	target := targetCopies(ownerID)
	var existing Blob
	if err := DB.Where("dedup_key = ?", dk).First(&existing).Error; err == nil {
		updates := map[string]any{"ref_count": existing.RefCount + 1}
		if target > existing.TargetCopies {
			updates["target_copies"] = target
			existing.TargetCopies = target
		}
		DB.Model(&Blob{}).Where("id = ?", existing.ID).Updates(updates)
		existing.RefCount++
		return &existing, nil
	}
	s.rw.RLock()
	defer s.rw.RUnlock()
	encrypted := scope == "private"
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	storedPath := filepath.Join(s.cfg.TmpDir, "stored-"+uuid.NewString())
	if err := compressToFile(plainPath, storedPath, level, encrypted, key, nonce); err != nil {
		return nil, err
	}
	defer os.Remove(storedPath)
	st, err := os.Stat(storedPath)
	if err != nil {
		return nil, err
	}
	packPath, err := s.ensurePack(scope, ownerID)
	if err != nil {
		return nil, err
	}
	lk := s.lockFor(packPath)
	lk.Lock()
	defer lk.Unlock()
	pf, err := os.OpenFile(packPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	offset, err := pf.Seek(0, io.SeekEnd)
	if err != nil {
		pf.Close()
		return nil, err
	}
	if offset == 0 {
		if _, err := pf.WriteString(packMagic); err != nil {
			pf.Close()
			return nil, err
		}
		offset = int64(len(packMagic))
	}
	hdr := make([]byte, recordHdr)
	copy(hdr[0:4], recordMagic)
	if encrypted {
		hdr[4] = 1
	}
	copy(hdr[5:17], nonce)
	binary.BigEndian.PutUint64(hdr[17:25], uint64(size))
	binary.BigEndian.PutUint64(hdr[25:33], uint64(st.Size()))
	raw, _ := hex.DecodeString(sum)
	copy(hdr[33:65], raw)
	if _, err := pf.Write(hdr); err != nil {
		pf.Close()
		return nil, err
	}
	sf, err := os.Open(storedPath)
	if err != nil {
		pf.Close()
		return nil, err
	}
	_, err = io.Copy(pf, sf)
	sf.Close()
	if err != nil {
		pf.Close()
		return nil, err
	}
	if err := pf.Sync(); err != nil {
		pf.Close()
		return nil, err
	}
	if err := pf.Close(); err != nil {
		return nil, err
	}
	blob := &Blob{
		ID:           uuid.NewString(),
		PackID:       s.packID(scope, ownerID),
		Offset:       offset,
		StoredSize:   st.Size(),
		PlainSize:    size,
		SHA256:       sum,
		DedupKey:     dk,
		Encrypted:    encrypted,
		RefCount:     1,
		TargetCopies: target,
		CreatedAt:    time.Now(),
	}
	if err := DB.Create(blob).Error; err != nil {
		return nil, err
	}
	DB.Model(&Pack{}).Where("id = ?", blob.PackID).Updates(map[string]any{"size": offset + recordHdr + st.Size(), "updated_at": time.Now()})
	return blob, nil
}

func compressToFile(src, dst string, level int, encrypted bool, key, baseNonce []byte) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	var w io.Writer = out
	var enc *chunkEncryptWriter
	if encrypted {
		if len(key) != 32 {
			return errors.New("private key must be 32 bytes")
		}
		enc, err = newChunkEncryptWriter(out, key, baseNonce)
		if err != nil {
			return err
		}
		w = enc
	}
	zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
	if err != nil {
		return err
	}
	if _, err := io.Copy(zw, in); err != nil {
		zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if enc != nil {
		if err := enc.Close(); err != nil {
			return err
		}
	}
	return out.Sync()
}

type chunkEncryptWriter struct {
	w     io.Writer
	aead  cipher.AEAD
	base  [12]byte
	buf   []byte
	count uint32
}

func newChunkEncryptWriter(w io.Writer, key, base []byte) (*chunkEncryptWriter, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	c := &chunkEncryptWriter{w: w, aead: aead, buf: make([]byte, 0, chunkSize)}
	if len(base) != 12 {
		return nil, errors.New("bad nonce")
	}
	copy(c.base[:], base)
	return c, nil
}

func (c *chunkEncryptWriter) nonce() []byte {
	n := c.base
	binary.BigEndian.PutUint32(n[8:], binary.BigEndian.Uint32(c.base[8:])^c.count)
	return n[:]
}

func (c *chunkEncryptWriter) flush() error {
	if len(c.buf) == 0 {
		return nil
	}
	ct := c.aead.Seal(nil, c.nonce(), c.buf, nil)
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(ct)))
	if _, err := c.w.Write(l[:]); err != nil {
		return err
	}
	if _, err := c.w.Write(ct); err != nil {
		return err
	}
	c.count++
	c.buf = c.buf[:0]
	return nil
}

func (c *chunkEncryptWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n := chunkSize - len(c.buf)
		if n > len(p) {
			n = len(p)
		}
		c.buf = append(c.buf, p[:n]...)
		p = p[n:]
		written += n
		if len(c.buf) == chunkSize {
			if err := c.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (c *chunkEncryptWriter) Close() error {
	return c.flush()
}

type chunkDecryptReader struct {
	r     io.Reader
	aead  cipher.AEAD
	base  [12]byte
	count uint32
	buf   []byte
	eof   bool
}

func newChunkDecryptReader(r io.Reader, key []byte, base []byte) (*chunkDecryptReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	c := &chunkDecryptReader{r: r, aead: aead}
	copy(c.base[:], base)
	return c, nil
}

func (c *chunkDecryptReader) nonce() []byte {
	n := c.base
	binary.BigEndian.PutUint32(n[8:], binary.BigEndian.Uint32(c.base[8:])^c.count)
	return n[:]
}

func (c *chunkDecryptReader) fill() error {
	if c.eof {
		return io.EOF
	}
	var l [4]byte
	if _, err := io.ReadFull(c.r, l[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			c.eof = true
			return io.EOF
		}
		return err
	}
	n := binary.BigEndian.Uint32(l[:])
	if n == 0 || n > chunkSize+64 {
		return errors.New("bad encrypted chunk")
	}
	ct := make([]byte, n)
	if _, err := io.ReadFull(c.r, ct); err != nil {
		return err
	}
	pt, err := c.aead.Open(nil, c.nonce(), ct, nil)
	if err != nil {
		return err
	}
	c.count++
	c.buf = pt
	return nil
}

func (c *chunkDecryptReader) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		if err := c.fill(); err != nil {
			return 0, err
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

type blobReadCloser struct {
	f    *os.File
	dec  *zstd.Decoder
	body io.Reader
}

func (b *blobReadCloser) Read(p []byte) (int, error) { return b.body.Read(p) }
func (b *blobReadCloser) Close() error {
	if b.dec != nil {
		b.dec.Close()
	}
	return b.f.Close()
}

func (s *PackStore) openBlob(blob *Blob, key []byte) (io.ReadCloser, error) {
	s.rw.RLock()
	defer s.rw.RUnlock()
	path := s.packPath("shared", 0)
	scope := "shared"
	owner := uint(0)
	if blob.PackID != "shared" {
		scope = "private"
		if _, err := fmt.Sscanf(blob.PackID, "private-%d", &owner); err != nil {
			return nil, err
		}
	}
	path = s.packPath(scope, owner)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	hdr := make([]byte, recordHdr)
	if _, err := f.ReadAt(hdr, blob.Offset); err != nil {
		f.Close()
		return nil, err
	}
	if string(hdr[0:4]) != recordMagic {
		f.Close()
		return nil, errors.New("bad blob header")
	}
	enc := hdr[4]&1 == 1
	nonce := hdr[5:17]
	var body io.Reader = io.NewSectionReader(f, blob.Offset+recordHdr, blob.StoredSize)
	if enc {
		if len(key) != 32 {
			f.Close()
			return nil, errors.New("missing private key")
		}
		dr, err := newChunkDecryptReader(body, key, nonce)
		if err != nil {
			f.Close()
			return nil, err
		}
		body = dr
	}
	zr, err := zstd.NewReader(body)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &blobReadCloser{f: f, dec: zr, body: zr}, nil
}

// RawOpen returns the exact stored bytes (zstd/AES envelope) for replication
// nodes. Nodes never receive the private key, so private data stays opaque.
func (s *PackStore) RawOpen(blobID string) (io.ReadCloser, error) {
	var b Blob
	if err := DB.First(&b, "id = ?", blobID).Error; err != nil {
		return nil, err
	}
	s.rw.RLock()
	defer s.rw.RUnlock()
	scope := "shared"
	owner := uint(0)
	if b.PackID != "shared" {
		scope = "private"
		if _, err := fmt.Sscanf(b.PackID, "private-%d", &owner); err != nil {
			return nil, err
		}
	}
	f, err := os.Open(s.packPath(scope, owner))
	if err != nil {
		return nil, err
	}
	return &sectionReadCloser{Reader: io.NewSectionReader(f, b.Offset+recordHdr, b.StoredSize), f: f}, nil
}

type sectionReadCloser struct {
	io.Reader
	f *os.File
}

func (s *sectionReadCloser) Close() error { return s.f.Close() }

func (s *PackStore) Open(blobID string, key []byte) (io.ReadCloser, error) {
	var b Blob
	if err := DB.First(&b, "id = ?", blobID).Error; err != nil {
		return nil, err
	}
	return s.openBlob(&b, key)
}

func (s *PackStore) ReadAll(blobID string, key []byte) ([]byte, error) {
	r, err := s.Open(blobID, key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// Materialize returns a local path that can be served with Range support.
// For private blobs the caller must remove the returned temp file after serving.
func (s *PackStore) Materialize(blobID string, key []byte) (string, bool, error) {
	var b Blob
	if err := DB.First(&b, "id = ?", blobID).Error; err != nil {
		return "", false, err
	}
	if b.Encrypted {
		path := filepath.Join(s.cfg.TmpDir, "dec-"+uuid.NewString())
		r, err := s.openBlob(&b, key)
		if err != nil {
			return "", false, err
		}
		defer r.Close()
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return "", false, err
		}
		if _, err := io.Copy(f, r); err != nil {
			f.Close()
			os.Remove(path)
			return "", false, err
		}
		if err := f.Close(); err != nil {
			os.Remove(path)
			return "", false, err
		}
		return path, true, nil
	}
	path := filepath.Join(s.cfg.CacheDir, b.ID)
	if st, err := os.Stat(path); err == nil && st.Size() == b.PlainSize {
		_ = os.Chtimes(path, time.Now(), time.Now())
		return path, false, nil
	}
	part := path + ".part"
	r, err := s.openBlob(&b, key)
	if err != nil {
		return "", false, err
	}
	defer r.Close()
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", false, err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(part)
		return "", false, err
	}
	if err := f.Close(); err != nil {
		os.Remove(part)
		return "", false, err
	}
	if err := os.Rename(part, path); err != nil {
		os.Remove(part)
		return "", false, err
	}
	s.trimCache()
	return path, false, nil
}

func (s *PackStore) trimCache() {
	entries, err := os.ReadDir(s.cfg.CacheDir)
	if err != nil {
		return
	}
	var total int64
	type ent struct {
		path string
		size int64
		at   time.Time
	}
	var list []ent
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		list = append(list, ent{filepath.Join(s.cfg.CacheDir, e.Name()), info.Size(), info.ModTime()})
	}
	if total <= s.cfg.CacheMaxBytes {
		return
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].at.Before(list[i].at) {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	for _, e := range list {
		if total <= s.cfg.CacheMaxBytes*8/10 {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
		}
	}
}

func (s *PackStore) Delete(blobID string) error {
	var b Blob
	if err := DB.First(&b, "id = ?", blobID).Error; err != nil {
		return err
	}
	if b.RefCount > 1 {
		return DB.Model(&Blob{}).Where("id = ?", b.ID).UpdateColumn("ref_count", b.RefCount-1).Error
	}
	if err := DB.Delete(&b).Error; err != nil {
		return err
	}
	return DB.Model(&Pack{}).Where("id = ?", b.PackID).Updates(map[string]any{"waste": gorm.Expr("waste + ?", b.StoredSize), "updated_at": time.Now()}).Error
}

// Compact rewrites one archive, dropping the bytes of deleted blobs and
// rewriting the offsets of live blobs. It is an explicit maintenance action
// because it rewrites the whole pack.
func (s *PackStore) Compact(packID string) (int64, int64, error) {
	s.rw.Lock()
	defer s.rw.Unlock()
	var p Pack
	if err := DB.First(&p, "id = ?", packID).Error; err != nil {
		return 0, 0, err
	}
	path := s.packPath("shared", 0)
	if p.Scope == "private" {
		path = s.packPath("private", p.OwnerID)
	}
	before, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	var blobs []Blob
	if err := DB.Where("pack_id = ?", packID).Order("offset ASC").Find(&blobs).Error; err != nil {
		return 0, 0, err
	}
	sort.Slice(blobs, func(i, j int) bool { return blobs[i].Offset < blobs[j].Offset })
	in, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer in.Close()
	tmp := path + ".compact"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, 0, err
	}
	cleanup := func() {
		out.Close()
		os.Remove(tmp)
	}
	if _, err := out.WriteString(packMagic); err != nil {
		cleanup()
		return 0, 0, err
	}
	offset := int64(len(packMagic))
	newOffsets := make(map[string]int64, len(blobs))
	for _, b := range blobs {
		hdr := make([]byte, recordHdr)
		if _, err := in.ReadAt(hdr, b.Offset); err != nil {
			cleanup()
			return 0, 0, err
		}
		if string(hdr[0:4]) != recordMagic {
			cleanup()
			return 0, 0, fmt.Errorf("bad record at %d", b.Offset)
		}
		if _, err := out.Write(hdr); err != nil {
			cleanup()
			return 0, 0, err
		}
		if _, err := io.CopyN(out, io.NewSectionReader(in, b.Offset+recordHdr, b.StoredSize), b.StoredSize); err != nil {
			cleanup()
			return 0, 0, err
		}
		newOffsets[b.ID] = offset
		offset += recordHdr + b.StoredSize
	}
	if err := out.Sync(); err != nil {
		cleanup()
		return 0, 0, err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return 0, 0, err
	}
	tx := DB.Begin()
	for id, off := range newOffsets {
		if err := tx.Model(&Blob{}).Where("id = ?", id).UpdateColumn("offset", off).Error; err != nil {
			tx.Rollback()
			os.Remove(tmp)
			return 0, 0, err
		}
	}
	if err := tx.Model(&Pack{}).Where("id = ?", packID).Updates(map[string]any{"size": offset, "waste": 0, "updated_at": time.Now()}).Error; err != nil {
		tx.Rollback()
		os.Remove(tmp)
		return 0, 0, err
	}
	if err := tx.Commit().Error; err != nil {
		os.Remove(tmp)
		return 0, 0, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return 0, 0, err
	}
	return before.Size(), offset, nil
}
