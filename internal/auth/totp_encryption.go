package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
)

var errTOTPEncryption = errors.New("TOTP encryption unavailable or invalid")

// TOTPEncryption protects stored secrets with the configured key and binds them
// to their player. An empty key disables new setups until the operator sets one.
type TOTPEncryption struct {
	keys   map[string]cipher.AEAD
	aead   cipher.AEAD
	keyID  string
	random io.Reader
}

// NewTOTPEncryption validates a raw 32-byte AES key. Keys never enter the database.
func NewTOTPEncryption(key []byte) (*TOTPEncryption, error) {
	e := &TOTPEncryption{random: rand.Reader}
	if len(key) == 0 {
		return e, nil
	}
	if len(key) != 32 {
		return nil, errTOTPEncryption
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errTOTPEncryption
	}
	e.aead, err = cipher.NewGCM(block)
	if err != nil {
		return nil, errTOTPEncryption
	}
	sum := sha256.Sum256(key)
	e.keyID = hex.EncodeToString(sum[:4])
	return e, nil
}

func (e *TOTPEncryption) encrypt(playerID int64, secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	if e.aead == nil {
		return "", errTOTPEncryption
	}
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(e.random, nonce); err != nil {
		return "", errTOTPEncryption
	}
	sealed := e.aead.Seal(nonce, nonce, []byte(secret), []byte(strconv.FormatInt(playerID, 10)))
	return "v1:" + e.keyID + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

func (e *TOTPEncryption) decrypt(playerID int64, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	parts := strings.Split(stored, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", errTOTPEncryption
	}
	aead := e.aead
	if parts[1] != e.keyID {
		aead = e.keys[parts[1]]
	}
	if aead == nil {
		return "", errTOTPEncryption
	}
	sealed, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return "", errTOTPEncryption
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(strconv.FormatInt(playerID, 10)))
	if err != nil {
		return "", errTOTPEncryption
	}
	return string(plain), nil
}

// NewTOTPEncryptionWithPrevious allows decryption with earlier keys during rotation.
func NewTOTPEncryptionWithPrevious(current []byte, previous [][]byte) (*TOTPEncryption, error) {
	e, err := NewTOTPEncryption(current)
	if err != nil {
		return nil, err
	}
	if len(previous) > 0 && e.aead == nil {
		return nil, errTOTPEncryption
	}
	e.keys = make(map[string]cipher.AEAD)
	for _, key := range previous {
		old, err := NewTOTPEncryption(key)
		if err != nil || old.aead == nil {
			return nil, errTOTPEncryption
		}
		if old.keyID == e.keyID {
			continue
		}
		if _, duplicate := e.keys[old.keyID]; duplicate {
			return nil, errTOTPEncryption
		}
		e.keys[old.keyID] = old.aead
	}
	return e, nil
}
