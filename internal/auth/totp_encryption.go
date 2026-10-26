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
	if e.aead == nil {
		return "", errTOTPEncryption
	}
	parts := strings.Split(stored, ":")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] != e.keyID {
		return "", errTOTPEncryption
	}
	sealed, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil || len(sealed) < e.aead.NonceSize()+e.aead.Overhead() {
		return "", errTOTPEncryption
	}
	plain, err := e.aead.Open(nil, sealed[:e.aead.NonceSize()], sealed[e.aead.NonceSize():], []byte(strconv.FormatInt(playerID, 10)))
	if err != nil {
		return "", errTOTPEncryption
	}
	return string(plain), nil
}
