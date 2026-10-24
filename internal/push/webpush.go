package push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// This file implements the two halves of Web Push that the server needs, with
// the standard library only (owner 2026-10-24, ADR 102): message encryption
// (RFC 8291, aes128gcm from RFC 8188) and VAPID authorization (RFC 8292).

const (
	recordSize   = 4096
	saltLength   = 16
	keyLength    = 65
	authLength   = 16
	vapidTTL     = 12 * time.Hour
	maxPlaintext = 3000 // well below the 4096-byte body limit of push services
)

var errInvalidKey = errors.New("invalid push key")

// VAPIDKey is the server's application server key pair (RFC 8292).
type VAPIDKey struct {
	private *ecdsa.PrivateKey
	public  []byte
	subject string
}

// ParseVAPIDKey reads the base64url private scalar from VAPID_PRIVATE_KEY and
// the contact subject (mailto: or https:) from VAPID_SUBJECT.
func ParseVAPIDKey(privateKey, subject string) (*VAPIDKey, error) {
	raw, err := decodeBase64URL(privateKey)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("VAPID_PRIVATE_KEY must be a base64url P-256 private key of 32 bytes")
	}
	priv, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, errors.New("VAPID_PRIVATE_KEY is not a valid P-256 private key")
	}
	if !strings.HasPrefix(subject, "mailto:") && !strings.HasPrefix(subject, "https://") {
		return nil, errors.New("VAPID_SUBJECT must start with mailto: or https://")
	}
	public, err := priv.PublicKey.Bytes()
	if err != nil {
		return nil, fmt.Errorf("VAPID public key: %w", err)
	}
	return &VAPIDKey{private: priv, public: public, subject: subject}, nil
}

// GenerateVAPIDKey returns a new private key in the format ParseVAPIDKey reads.
func GenerateVAPIDKey() (string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	raw, err := priv.Bytes()
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// PublicKey is the base64url uncompressed public key the browser needs as
// applicationServerKey.
func (k *VAPIDKey) PublicKey() string {
	return base64.RawURLEncoding.EncodeToString(k.public)
}

// authorization builds the Authorization header value for one endpoint.
func (k *VAPIDKey) authorization(endpoint string, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{"aud": u.Scheme + "://" + u.Host, "exp": now.Add(vapidTTL).Unix(), "sub": k.subject})
	if err != nil {
		return "", err
	}
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, k.private, digest[:])
	if err != nil {
		return "", err
	}
	// JWS ES256 wants the raw 64-byte r||s, not ASN.1.
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	return "vapid t=" + token + ", k=" + k.PublicKey(), nil
}

// encrypt seals plaintext for one subscription (RFC 8291 section 3). The
// random sources are parameters so the RFC test vector can be reproduced.
func encrypt(plaintext, uaPublic, authSecret []byte, serverKey *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext) > maxPlaintext {
		return nil, errors.New("push payload too large")
	}
	if len(authSecret) != authLength || len(salt) != saltLength {
		return nil, errInvalidKey
	}
	uaKey, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, errInvalidKey
	}
	sharedSecret, err := serverKey.ECDH(uaKey)
	if err != nil {
		return nil, err
	}
	asPublic := serverKey.PublicKey().Bytes()
	keyInfo := "WebPush: info\x00" + string(uaPublic) + string(asPublic)
	ikm, err := hkdf.Key(sha256.New, sharedSecret, authSecret, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	contentKey, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// One record: the payload followed by the last-record delimiter 0x02.
	record := append(append([]byte{}, plaintext...), 0x02)
	var body bytes.Buffer
	body.Write(salt)
	_ = binary.Write(&body, binary.BigEndian, uint32(recordSize))
	body.WriteByte(byte(len(asPublic)))
	body.Write(asPublic)
	body.Write(gcm.Seal(nil, nonce, record, nil))
	return body.Bytes(), nil
}

// encryptRandom is encrypt with a fresh server key and salt per message.
func encryptRandom(plaintext, uaPublic, authSecret []byte, random io.Reader) ([]byte, error) {
	serverKey, err := ecdh.P256().GenerateKey(random)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, saltLength)
	if _, err := io.ReadFull(random, salt); err != nil {
		return nil, err
	}
	return encrypt(plaintext, uaPublic, authSecret, serverKey, salt)
}

// decodeBase64URL accepts the padded and unpadded base64url forms browsers use.
func decodeBase64URL(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(strings.TrimSpace(value), "="))
}
