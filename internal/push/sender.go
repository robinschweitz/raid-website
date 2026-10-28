package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// sendTimeout bounds one whole fan-out, so a slow push service cannot hold
	// the request that triggered it for long (SC-026 WriteTimeout is 60 s).
	sendTimeout = 15 * time.Second
	// maxParallelSends keeps the fan-out small on the single host.
	maxParallelSends = 4
	// messageTTL lets a push service hold a message for a phone that is off.
	messageTTL = "86400"
)

func ecdhPublicKey(raw []byte) (*ecdh.PublicKey, error) {
	return ecdh.P256().NewPublicKey(raw)
}

type deliveryResult struct {
	sub    Subscription
	status int
	err    error
}

// deliver sends payload to every subscription with at most maxParallelSends
// requests at a time and waits for all of them (CODING_RULES 8.3).
func (s *Service) deliver(ctx context.Context, subs []Subscription, kind Kind, payload []byte) []deliveryResult {
	results := make([]deliveryResult, len(subs))
	slots := make(chan struct{}, maxParallelSends)
	var wg sync.WaitGroup
	for i, sub := range subs {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			status, err := s.sender.Send(ctx, sub, kind, payload)
			results[i] = deliveryResult{sub: sub, status: status, err: err}
		}()
	}
	wg.Wait()
	return results
}

// HTTPSender posts encrypted messages to push services (RFC 8030).
type HTTPSender struct {
	key    *VAPIDKey
	client *http.Client
	now    func() time.Time
}

// NewHTTPSender builds a sender that never follows redirects, so a push
// service cannot send the server to another host.
func NewHTTPSender(key *VAPIDKey) *HTTPSender {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &HTTPSender{key: key, client: client, now: time.Now}
}

// Send encrypts payload for sub and posts it. Topic lets the push service
// replace an undelivered message of the same kind instead of queueing both.
func (h *HTTPSender) Send(ctx context.Context, sub Subscription, kind Kind, payload []byte) (int, error) {
	uaPublic, err := decodeBase64URL(sub.P256dh)
	if err != nil {
		return 0, errInvalidKey
	}
	authSecret, err := decodeBase64URL(sub.Auth)
	if err != nil {
		return 0, errInvalidKey
	}
	body, err := encryptRandom(payload, uaPublic, authSecret, rand.Reader)
	if err != nil {
		return 0, err
	}
	authorization, err := h.key.authorization(sub.Endpoint, h.now())
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", messageTTL)
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Topic", string(kind))
	req.Header.Set("Authorization", authorization)
	resp, err := h.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// The answer is not used; read a little so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}
