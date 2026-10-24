package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Argon2id parameters for new password hashes (SC-005, ADR 100): 64 MiB,
// 3 passes, one lane, 16-byte salt, 32-byte key.
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 1
	argonSaltLen        = 16
	argonKeyLen  uint32 = 32

	// maxPasswordBytes bounds the hashing work per request (SC-005). Argon2id
	// has no input limit of its own, so the bound is a policy, not a cut-off.
	maxPasswordBytes = 1024
)

// argonPrefix is the PHC prefix of a hash made with the current parameters;
// a stored hash without it is rehashed after the next successful login.
var argonPrefix = fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$", argon2.Version, argonMemory, argonTime, argonThreads)

// argonSlots caps how many Argon2id computations run at once, so parallel
// logins cannot claim more than slots × 64 MiB of memory on the host.
var argonSlots = make(chan struct{}, 2)

var errPasswordMismatch = errors.New("password mismatch")

func argonKey(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
	argonSlots <- struct{}{}
	defer func() { <-argonSlots }()
	return argon2.IDKey(password, salt, time, memory, threads, keyLen)
}

// HashPassword derives an Argon2id hash in PHC string format for storage.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argonKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	encoding := base64.RawStdEncoding
	return argonPrefix + encoding.EncodeToString(salt) + "$" + encoding.EncodeToString(key), nil
}

// needsRehash reports whether a stored hash is bcrypt or Argon2id with
// parameters other than the current ones.
func needsRehash(hash string) bool {
	return !strings.HasPrefix(hash, argonPrefix)
}

// verifyStoredPassword checks a password against an Argon2id PHC hash or a
// legacy bcrypt hash. Parameters outside sane bounds are rejected, so a
// corrupted row cannot make one login allocate unbounded memory.
func verifyStoredPassword(hash, password string) error {
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return errors.New("invalid password hash")
	}
	var memory, time uint32
	var threads uint8
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil || n != 3 ||
		fmt.Sprintf("m=%d,t=%d,p=%d", memory, time, threads) != parts[3] {
		return errors.New("invalid argon2 parameters")
	}
	if threads == 0 || threads > 8 || memory < 8*uint32(threads) || memory > 256*1024 || time == 0 || time > 10 {
		return errors.New("invalid argon2 parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return errors.New("invalid argon2 salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return errors.New("invalid argon2 key")
	}
	actual := argonKey([]byte(password), salt, time, memory, threads, uint32(len(expected)))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return errPasswordMismatch
	}
	return nil
}
