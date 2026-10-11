package registration

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"raid-website/internal/character"
)

// risingGodsWarmConcurrency keeps the background warm-up gentle on the
// Rising-Gods database, which spends several seconds on every list search.
const risingGodsWarmConcurrency = 2

// RunSearchCacheWarmer refreshes the search cache for every active character
// name now and then every interval, until ctx is cancelled. Searches for known
// characters (claims, quick-add, profile reloads) are then answered from cache.
func RunSearchCacheWarmer(ctx context.Context, chars *character.Repo, client RisingGodsClient, interval time.Duration) {
	for {
		list, err := chars.List(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("rising-gods cache warm: list characters: %v", err)
		} else {
			names := warmSearchNames(list)
			warmed, failed := WarmSearchCache(ctx, client, names, risingGodsWarmConcurrency)
			log.Printf("rising-gods cache warm: names=%d warmed=%d failed=%d", len(names), warmed, failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// WarmSearchCache makes sure each name has a fresh cached search, running at
// most concurrency upstream requests at a time.
func WarmSearchCache(ctx context.Context, client RisingGodsClient, names []string, concurrency int) (warmed, failed int) {
	if concurrency < 1 {
		concurrency = 1
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for _, name := range names {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return warmed, failed
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			err := client.WarmSearch(ctx, name)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			warmed++
		}()
	}
	wg.Wait()
	return warmed, failed
}

func warmSearchNames(chars []character.Character) []string {
	seen := make(map[string]bool, len(chars))
	names := make([]string, 0, len(chars))
	for _, c := range chars {
		name := strings.TrimSpace(c.Name)
		key := strings.ToLower(name)
		if !c.Active || len([]rune(name)) < 3 || seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, name)
	}
	return names
}
