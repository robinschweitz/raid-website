package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	wan "github.com/go-webauthn/webauthn/webauthn"

	"raid-website/internal/player"
)

const (
	cloneTestRPID   = "localhost"
	cloneTestOrigin = "http://localhost"
)

// cloneTestKey is a software authenticator with an ES256 key.
type cloneTestKey struct {
	id  []byte
	key *ecdsa.PrivateKey
}

func newCloneTestKey(t *testing.T) cloneTestKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return cloneTestKey{id: []byte("clone-test-credential"), key: key}
}

// coseKey encodes the public key as a COSE EC2 key (CBOR map of five entries).
func (k cloneTestKey) coseKey(t *testing.T) []byte {
	t.Helper()
	point, err := k.key.PublicKey.Bytes() // uncompressed: 0x04 || X || Y
	if err != nil {
		t.Fatal(err)
	}
	x, y := point[1:33], point[33:65]
	out := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	out = append(out, x...)
	out = append(out, 0x22, 0x58, 0x20)
	return append(out, y...)
}

// assertion builds a login response body with the given signature counter.
func (k cloneTestKey) assertion(t *testing.T, challenge string, userID int64, counter uint32) []byte {
	t.Helper()
	enc := base64.RawURLEncoding
	clientData, err := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": challenge, "origin": cloneTestOrigin})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte(cloneTestRPID))
	authData := append([]byte{}, rpHash[:]...)
	authData = append(authData, 0x05) // user present and user verified
	authData = binary.BigEndian.AppendUint32(authData, counter)
	clientHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), clientHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"id":    enc.EncodeToString(k.id),
		"rawId": enc.EncodeToString(k.id),
		"type":  "public-key",
		"response": map[string]any{
			"authenticatorData": enc.EncodeToString(authData),
			"clientDataJSON":    enc.EncodeToString(clientData),
			"signature":         enc.EncodeToString(signature),
			"userHandle":        enc.EncodeToString([]byte(strconv.FormatInt(userID, 10))),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func storedSignCount(t *testing.T, svc *Service, playerID int64) uint32 {
	t.Helper()
	rows, err := svc.sessions.ListWebAuthnCredentials(context.Background(), playerID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListWebAuthnCredentials = %d rows, %v", len(rows), err)
	}
	var credential wan.Credential
	if err := json.Unmarshal([]byte(rows[0].CredentialJSON), &credential); err != nil {
		t.Fatal(err)
	}
	return credential.Authenticator.SignCount
}

// beginKeyLogin starts a password login and the WebAuthn step, and returns
// the WebAuthn session token and the challenge to sign.
func beginKeyLogin(t *testing.T, svc *Service) (string, string) {
	t.Helper()
	challenge := startMFALogin(t, svc)
	begin, err := svc.BeginWebAuthnMFA(context.Background(), challenge, cloneTestRPID, cloneTestOrigin)
	if err != nil {
		t.Fatal(err)
	}
	return begin.Session, begin.Options.Response.Challenge.String()
}

// TestReqAuth021_AC01 covers SC-048: a lower counter is rejected and the
// stored counter stays; a higher counter still logs in.
func TestReqAuth021_AC01(t *testing.T) {
	ctx := context.Background()
	bdb, svc := loginLimitService(t)
	owner, err := player.NewRepo(bdb.DB).FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	key := newCloneTestKey(t)
	credentialJSON, err := json.Marshal(wan.Credential{ID: key.id, PublicKey: key.coseKey(t), Authenticator: wan.Authenticator{SignCount: 10}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := svc.sessions.CreateWebAuthnCredential(ctx, &WebAuthnCredential{PlayerID: owner.ID, CredentialIDHash: hashBytes(key.id), Name: "Test key", CredentialJSON: string(credentialJSON), CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	session, challenge := beginKeyLogin(t, svc)
	sessionsBefore := countSessions(t, bdb)
	_, _, err = svc.FinishWebAuthnMFA(ctx, session, key.assertion(t, challenge, owner.ID, 5), cloneTestRPID, cloneTestOrigin)
	if !errors.Is(err, ErrInvalidMFA) || !errors.Is(err, ErrClonedSecurityKey) {
		t.Fatalf("lower counter: err = %v, want invalid MFA and cloned key", err)
	}
	if id, ok := PlayerIDFromError(err); !ok || id != owner.ID {
		t.Fatalf("PlayerIDFromError = %d, %v; want %d", id, ok, owner.ID)
	}
	if got := storedSignCount(t, svc, owner.ID); got != 10 {
		t.Fatalf("stored counter after rejection = %d, want 10", got)
	}
	if got := countSessions(t, bdb); got != sessionsBefore {
		t.Fatalf("sessions after rejection = %d, want %d", got, sessionsBefore)
	}

	// Positive control: the same key with a higher counter logs in and the
	// counter is stored.
	session, challenge = beginKeyLogin(t, svc)
	token, _, err := svc.FinishWebAuthnMFA(ctx, session, key.assertion(t, challenge, owner.ID, 11), cloneTestRPID, cloneTestOrigin)
	if err != nil || token == "" {
		t.Fatalf("higher counter: token %q, err %v; want a session", token, err)
	}
	if got := storedSignCount(t, svc, owner.ID); got != 11 {
		t.Fatalf("stored counter after login = %d, want 11", got)
	}
}
