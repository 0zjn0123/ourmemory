package om

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"
)

func masterAEAD() (cipher.AEAD, error) {
	block, err := aes.NewCipher(Cfg.MasterKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func wrapUserKey(key []byte) (string, error) {
	aead, err := masterAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := aead.Seal(nil, nonce, key, nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

func unwrapUserKey(s string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	aead, err := masterAEAD()
	if err != nil {
		return nil, err
	}
	if len(raw) < aead.NonceSize() {
		return nil, errors.New("bad wrapped key")
	}
	nonce, ct := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	return aead.Open(nil, nonce, ct, nil)
}

// UserKey returns the 32-byte private archive key, creating it on first use.
func UserKey(userID uint) ([]byte, error) {
	if userID == 0 {
		return nil, errors.New("invalid user")
	}
	var uk PrivateKey
	err := DB.First(&uk, "user_id = ?", userID).Error
	if err == nil {
		key, err := unwrapUserKey(uk.WrappedKey)
		if err != nil {
			return nil, err
		}
		if len(key) != 32 {
			return nil, errors.New("bad user key")
		}
		return key, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	wrapped, err := wrapUserKey(key)
	if err != nil {
		return nil, err
	}
	if err := DB.Create(&PrivateKey{UserID: userID, WrappedKey: wrapped, CreatedAt: time.Now()}).Error; err != nil {
		return nil, err
	}
	return key, nil
}

func EnablePrivate(userID uint) error {
	key, err := UserKey(userID)
	if err != nil {
		return err
	}
	_ = key
	_, err = Store.ensurePack("private", userID)
	if err != nil {
		return err
	}
	return DB.Model(&Profile{}).Where("user_id = ?", userID).Updates(map[string]any{
		"private_enabled": true,
		"updated_at":      time.Now(),
	}).Error
}
