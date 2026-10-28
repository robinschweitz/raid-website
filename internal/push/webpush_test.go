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
	"math/big"
	"strings"
	"testing"
	"time"
)

func mustB64(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := decodeBase64URL(value)
	if err != nil {
		t.Fatalf("decode %q: %v", value, err)
	}
	return raw
}

// RFC 8291 Appendix A.
const (
	rfcPlaintext  = "When I grow up, I want to be a watermelon"
	rfcASPrivate  = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcUAPrivate  = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcUAPublic   = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcSalt       = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcAuthSecret = "BTBZMqHH6r4Tts7J_aSIgg"
	rfcBody       = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func TestReqPush001_EncryptMatchesRFC8291Vector(t *testing.T) {
	serverKey, err := ecdh.P256().NewPrivateKey(mustB64(t, rfcASPrivate))
	if err != nil {
		t.Fatal(err)
	}
	body, err := encrypt([]byte(rfcPlaintext), mustB64(t, rfcUAPublic), mustB64(t, rfcAuthSecret), serverKey, mustB64(t, rfcSalt))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.RawURLEncoding.EncodeToString(body); got != rfcBody {
		t.Fatalf("body = %s\nwant   %s", got, rfcBody)
	}
}

// decryptForTest is the user agent side of RFC 8291, so a random message can
// be checked end to end.
func decryptForTest(t *testing.T, body, uaPrivateRaw, authSecret []byte) []byte {
	t.Helper()
	salt := body[:16]
	if rs := binary.BigEndian.Uint32(body[16:20]); rs != recordSize {
		t.Fatalf("record size %d", rs)
	}
	idLen := int(body[20])
	asPublic := body[21 : 21+idLen]
	ciphertext := body[21+idLen:]
	uaPrivate, err := ecdh.P256().NewPrivateKey(uaPrivateRaw)
	if err != nil {
		t.Fatal(err)
	}
	asKey, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := uaPrivate.ECDH(asKey)
	if err != nil {
		t.Fatal(err)
	}
	info := "WebPush: info\x00" + string(uaPrivate.PublicKey().Bytes()) + string(asPublic)
	ikm, _ := hkdf.Key(sha256.New, shared, authSecret, info, 32)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if plain[len(plain)-1] != 0x02 {
		t.Fatalf("missing last-record delimiter")
	}
	return plain[:len(plain)-1]
}

func TestReqPush001_RandomMessageDecrypts(t *testing.T) {
	uaPrivate := mustB64(t, rfcUAPrivate)
	authSecret := mustB64(t, rfcAuthSecret)
	body, err := encryptRandom([]byte(`{"kind":"poll"}`), mustB64(t, rfcUAPublic), authSecret, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := decryptForTest(t, body, uaPrivate, authSecret); string(got) != `{"kind":"poll"}` {
		t.Fatalf("plaintext = %q", got)
	}
	other, _ := encryptRandom([]byte(`{"kind":"poll"}`), mustB64(t, rfcUAPublic), authSecret, rand.Reader)
	if bytes.Equal(body[:16], other[:16]) {
		t.Fatal("salt was reused")
	}
}

func TestReqPush001_EncryptRejectsBadKeys(t *testing.T) {
	serverKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	salt := make([]byte, 16)
	if _, err := encrypt([]byte("x"), make([]byte, 65), mustB64(t, rfcAuthSecret), serverKey, salt); err == nil {
		t.Fatal("point not on the curve accepted")
	}
	if _, err := encrypt([]byte("x"), mustB64(t, rfcUAPublic), []byte("short"), serverKey, salt); err == nil {
		t.Fatal("short auth secret accepted")
	}
	if _, err := encrypt(make([]byte, maxPlaintext+1), mustB64(t, rfcUAPublic), mustB64(t, rfcAuthSecret), serverKey, salt); err == nil {
		t.Fatal("oversized payload accepted")
	}
}

func TestReqPush002_VAPIDAuthorizationVerifies(t *testing.T) {
	privateKey, err := GenerateVAPIDKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := ParseVAPIDKey(privateKey, "mailto:raid@example.org")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	header, err := key.authorization("https://fcm.googleapis.com/fcm/send/abc", now)
	if err != nil {
		t.Fatal(err)
	}
	token, publicKey, ok := strings.Cut(strings.TrimPrefix(header, "vapid t="), ", k=")
	if !ok || publicKey != key.PublicKey() {
		t.Fatalf("header = %q", header)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(mustB64(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://fcm.googleapis.com" || claims.Sub != "mailto:raid@example.org" || claims.Exp != now.Add(12*time.Hour).Unix() {
		t.Fatalf("claims = %+v", claims)
	}
	verifier, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), mustB64(t, publicKey))
	if err != nil {
		t.Fatal(err)
	}
	signature := mustB64(t, parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(verifier, digest[:], r, s) {
		t.Fatal("signature does not verify")
	}
}

func TestReqPush002_ParseVAPIDKeyRejectsBadConfig(t *testing.T) {
	good, _ := GenerateVAPIDKey()
	cases := map[string][2]string{
		"not base64":   {"!!!", "mailto:a@b.c"},
		"wrong length": {base64.RawURLEncoding.EncodeToString([]byte("short")), "mailto:a@b.c"},
		"bad subject":  {good, "raid@example.org"},
		"http subject": {good, "http://example.org"},
	}
	for name, input := range cases {
		if _, err := ParseVAPIDKey(input[0], input[1]); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ParseVAPIDKey(good, "https://raid.example.org"); err != nil {
		t.Fatalf("https subject rejected: %v", err)
	}
}

func TestReqPush003_EndpointAllowlist(t *testing.T) {
	p256dh, auth := rfcUAPublic, rfcAuthSecret
	ok := []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://web.push.apple.com/QABC",
		"https://wns2-par02p.notify.windows.com/w/?token=abc",
	}
	for _, endpoint := range ok {
		if err := (SubscriptionInput{Endpoint: endpoint, P256dh: p256dh, Auth: auth}).validate(); err != nil {
			t.Errorf("%s rejected: %v", endpoint, err)
		}
	}
	bad := []string{
		"http://fcm.googleapis.com/fcm/send/abc",
		"https://127.0.0.1/push",
		"https://localhost/push",
		"https://fcm.googleapis.com.evil.example/x",
		"https://evilpush.apple.com/x",
		"https://push.apple.com/x",
		"https://fcm.googleapis.com:8443/x",
		"https://user@fcm.googleapis.com/x",
		"https://example.org/fcm.googleapis.com",
		"",
		"https://fcm.googleapis.com/" + strings.Repeat("a", maxEndpointLength),
	}
	for _, endpoint := range bad {
		if err := (SubscriptionInput{Endpoint: endpoint, P256dh: p256dh, Auth: auth}).validate(); err == nil {
			t.Errorf("%q accepted", endpoint)
		}
	}
	endpoint := ok[0]
	if (SubscriptionInput{Endpoint: endpoint, P256dh: base64.RawURLEncoding.EncodeToString(make([]byte, 65)), Auth: auth}).validate() == nil {
		t.Error("invalid curve point accepted")
	}
	if (SubscriptionInput{Endpoint: endpoint, P256dh: p256dh, Auth: "AAAA"}).validate() == nil {
		t.Error("short auth accepted")
	}
}
