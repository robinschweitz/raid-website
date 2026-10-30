package registration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var ErrSearchLimit = errors.New("visitor search limit")

const visitorLimit = 10
const visitorWindow = 10 * time.Minute

type visitorWindowCount struct {
	started time.Time
	count   int
}
type visitorLimiter struct {
	mu      sync.Mutex
	clients map[string]visitorWindowCount
}

func (l *visitorLimiter) take(address string, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.clients == nil {
		l.clients = make(map[string]visitorWindowCount)
	}
	for key, value := range l.clients {
		if !now.Before(value.started.Add(visitorWindow)) {
			delete(l.clients, key)
		}
	}
	value := l.clients[address]
	if value.count >= visitorLimit {
		return ErrSearchLimit
	}
	if value.count == 0 {
		value.started = now
	}
	value.count++
	l.clients[address] = value
	return nil
}

// SearchForVisitor applies the shared visitor budget before any uncached lookup.
// Cached answers are served directly without refreshing on the visitor's behalf.
func (s *Service) SearchForVisitor(ctx context.Context, query, address string, authenticated bool) ([]ExternalCharacter, error) {
	if authenticated {
		return s.SearchCharacters(ctx, query)
	}
	if cache, ok := s.external.(interface {
		CachedSearch(string) ([]ExternalCharacter, bool)
	}); ok {
		if rows, found := cache.CachedSearch(query); found {
			return rows, nil
		}
	}
	if err := s.visitors.take(address, time.Now().UTC()); err != nil {
		return nil, err
	}
	return s.SearchCharacters(ctx, query)
}

// CreateVisitorRequest shares the search budget; authenticated callers are exempt.
func (s *Service) CreateVisitorRequest(ctx context.Context, profileID, address string, authenticated bool) (*Request, error) {
	if !authenticated {
		if err := s.visitors.take(address, time.Now().UTC()); err != nil {
			return nil, err
		}
	}
	return s.CreateRequest(ctx, nil, profileID)
}

// CachedSearch reports a reusable answer without starting an upstream request.
func (c RisingGodsClient) CachedSearch(query string) ([]ExternalCharacter, bool) {
	q := strings.TrimSpace(query)
	cache := c.sharedCache()
	if ref, ok := risingGodsProfileReference(q); ok {
		id := profileLoadID(ref)
		if id == "" {
			slug, err := c.normalizedRisingGodsProfileURL(ref)
			if err != nil {
				return nil, false
			}
			var found bool
			id, found = cache.getSlug("slug:" + strings.ToLower(slug))
			if !found {
				return nil, false
			}
		}
		profile, _, found := cache.getProfile("profile:" + id)
		if found {
			return []ExternalCharacter{profile}, true
		}
		return nil, false
	}
	if rows, _, ok := cache.getSearch("search:" + strings.ToLower(q)); ok {
		return rows, true
	}
	return cache.getCompletePrefixSearch(q)
}
