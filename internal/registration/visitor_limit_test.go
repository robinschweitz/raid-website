package registration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type limitedFeed struct {
	calls  atomic.Int32
	cached bool
}

func (f *limitedFeed) SearchCharacters(context.Context, string) ([]ExternalCharacter, error) {
	f.calls.Add(1)
	return []ExternalCharacter{}, nil
}
func (f *limitedFeed) LoadCharacter(context.Context, string) (ExternalCharacter, string, error) {
	return ExternalCharacter{}, "", ErrExternalNotFound
}
func (f *limitedFeed) CachedSearch(string) ([]ExternalCharacter, bool) {
	return []ExternalCharacter{}, f.cached
}

func TestReqAccount014_AC01(t *testing.T) {
	feed := &limitedFeed{}
	svc := &Service{external: feed}
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.SearchForVisitor(context.Background(), "Mage", "192.0.2.1", false)
			if err == nil {
				accepted.Add(1)
			} else if err != ErrSearchLimit {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 10 || feed.calls.Load() != 10 {
		t.Fatalf("accepted=%d upstream=%d", accepted.Load(), feed.calls.Load())
	}
	feed.cached = true
	if _, err := svc.SearchForVisitor(context.Background(), "Mage", "192.0.2.1", false); err != nil {
		t.Fatal("cached answer limited")
	}
	if feed.calls.Load() != 10 {
		t.Fatal("cache hit started upstream")
	}
}

func TestReqAccount014_AC02(t *testing.T) {
	feed := &limitedFeed{}
	svc := &Service{external: feed}
	for i := 0; i < 20; i++ {
		if _, err := svc.SearchForVisitor(context.Background(), "Mage", "192.0.2.1", true); err != nil {
			t.Fatal(err)
		}
	}
	if feed.calls.Load() != 20 {
		t.Fatal("authenticated search limited")
	}
}

func TestVisitorLimitExpiresAndSeparatesAddresses(t *testing.T) {
	var l visitorLimiter
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		if err := l.take("a", now); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.take("a", now.Add(visitorWindow-time.Second)); err != ErrSearchLimit {
		t.Fatal("window ended early")
	}
	if err := l.take("b", now); err != nil {
		t.Fatal("addresses share budget")
	}
	if err := l.take("a", now.Add(visitorWindow)); err != nil {
		t.Fatal("window did not expire")
	}
	if len(l.clients) != 1 {
		t.Fatal("expired addresses retained")
	}
}
