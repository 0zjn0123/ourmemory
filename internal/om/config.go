package om

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
)

// Config is intentionally environment-driven: the deployment file is the only
// place that holds secrets, and nothing here is ever written to logs.
type Config struct {
	DataDir         string
	PacksDir        string
	CacheDir        string
	TmpDir          string
	SharedMaxBytes  int64
	CacheMaxBytes   int64
	BaseURL         string
	BaseHost        string
	MasterKey       []byte
	SkyMailBase     string
	SkyMailEmail    string
	SkyMailPassword string
	SkyMailAccount  string
	DefaultQuota    int64
	NodeSecret      string
	Debug           bool
}

var Cfg Config

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func loadMasterKey(dataDir string) ([]byte, error) {
	if raw := strings.TrimSpace(os.Getenv("OM_MASTER_KEY")); raw != "" {
		if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
			return b, nil
		}
		if b, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
			return b, nil
		}
		if len(raw) == 64 {
			if b, err := hexDecode(raw); err == nil && len(b) == 32 {
				return b, nil
			}
		}
		return nil, fmt.Errorf("OM_MASTER_KEY must be 32 bytes, base64 or 64-char hex")
	}
	path := filepath.Join(dataDir, "master.key")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return b[:32], nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

func hexDecode(s string) ([]byte, error) {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi := strings.IndexByte("0123456789abcdef", s[i*2])
		lo := strings.IndexByte("0123456789abcdef", s[i*2+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("invalid hex")
		}
		out[i] = byte(hi<<4 | lo)
	}
	return out, nil
}

func loadConfig() (Config, error) {
	dataDir := conf.DataDir
	if dataDir == "" {
		dataDir = "data"
	}
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	key, err := loadMasterKey(dataDir)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		DataDir:         dataDir,
		PacksDir:        filepath.Join(dataDir, "packs"),
		CacheDir:        filepath.Join(dataDir, "cache"),
		TmpDir:          filepath.Join(dataDir, "tmp"),
		SharedMaxBytes:  envInt64("OM_SHARED_MAX_BYTES", 5*1024*1024*1024),
		CacheMaxBytes:   envInt64("OM_CACHE_BYTES", 10*1024*1024*1024),
		BaseURL:         strings.TrimRight(env("OM_BASE_URL", "http://127.0.0.1:5259"), "/"),
		BaseHost:        strings.TrimSpace(env("OM_BASE_HOST", "")),
		MasterKey:       key,
		SkyMailBase:     strings.TrimRight(env("OM_SKYMAIL_BASE", "https://skymail.ink"), "/"),
		SkyMailEmail:    env("OM_SKYMAIL_EMAIL", ""),
		SkyMailPassword: env("OM_SKYMAIL_PASSWORD", ""),
		SkyMailAccount:  env("OM_SKYMAIL_ACCOUNT_ID", ""),
		DefaultQuota:    envInt64("OM_DEFAULT_PRIVATE_QUOTA", 1024*1024*1024),
		NodeSecret:      env("OM_NODE_SECRET", ""),
		Debug:           env("OM_DEBUG", "") == "1",
	}
	for _, d := range []string{cfg.PacksDir, cfg.CacheDir, cfg.TmpDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}
