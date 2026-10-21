package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"9router/proxy/internal/db"
)

// RefreshConfig holds configuration for proxy pool auto-refresh
type RefreshConfig struct {
	Enabled          bool
	Interval         time.Duration // How often to refresh the proxy pool
	InitialDelay     time.Duration // Delay before first refresh
	TopN             int           // Number of top proxies to keep
	ProbeURL         string        // URL to use for speed testing
	Concurrency      int           // Concurrent speed tests
	PoolName         string        // Name for the auto-refresh pool
	PoolID           string        // Fixed ID for the auto-refresh pool (empty = generate new)
	RequestTimeout   time.Duration // Timeout for individual proxy requests
	SpeedTestTimeout time.Duration // Overall timeout for speed testing all proxies
}

// DefaultRefreshConfig returns sensible defaults for proxy pool auto-refresh
func DefaultRefreshConfig() RefreshConfig {
	return RefreshConfig{
		Enabled:          true,
		Interval:         1 * time.Hour,
		InitialDelay:     30 * time.Second,
		TopN:             15,
		ProbeURL:         "https://www.google.com/generate_204",
		Concurrency:      5,
		PoolName:         "Auto: Stormsia Residential",
		RequestTimeout:   10 * time.Second,
		SpeedTestTimeout: 5 * time.Minute,
	}
}

// ProxyRefresher manages automatic proxy pool refresh from stormsia.github.io
type ProxyRefresher struct {
	repo     *db.Repo
	config   RefreshConfig
	mu       sync.Mutex
	started  bool
	lastRun  time.Time
	lastErr  error
}

// NewProxyRefresher creates a new ProxyRefresher instance
func NewProxyRefresher(repo *db.Repo, config RefreshConfig) *ProxyRefresher {
	if config.TopN <= 0 {
		config.TopN = 15
	}
	if config.Interval <= 0 {
		config.Interval = 1 * time.Hour
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 30 * time.Second
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 5
	}
	if config.ProbeURL == "" {
		config.ProbeURL = "https://www.google.com/generate_204"
	}
	if config.PoolName == "" {
		config.PoolName = "Auto: Stormsia Residential"
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 10 * time.Second
	}
	if config.SpeedTestTimeout <= 0 {
		config.SpeedTestTimeout = 5 * time.Minute
	}

	return &ProxyRefresher{
		repo:   repo,
		config: config,
	}
}

// Start begins the background refresh loop.
// Safe to call multiple times; extra calls are no-ops.
func (r *ProxyRefresher) Start(ctx context.Context) {
	if !r.config.Enabled {
		return
	}

	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	go func() {
		// Wait for initial delay before first refresh
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.config.InitialDelay):
		}

		// Run first refresh immediately
		r.refreshOnce(ctx)

		ticker := time.NewTicker(r.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.refreshOnce(ctx)
			}
		}
	}()
}

// refreshOnce performs a single refresh cycle
func (r *ProxyRefresher) refreshOnce(ctx context.Context) {
	log.Println("[PROXY-FETCHER] starting proxy pool refresh")

	// Create a context with timeout for the entire refresh cycle
	refreshCtx, cancel := context.WithTimeout(ctx, r.config.SpeedTestTimeout)
	defer cancel()

	// 1. Fetch proxy list from stormsia
	proxies, err := FetchStormsiaProxies(refreshCtx, r.config.TopN)
	if err != nil {
		log.Printf("[PROXY-FETCHER] failed to fetch proxies: %v", err)
		r.setLastError(err)
		return
	}

	log.Printf("[PROXY-FETCHER] fetched %d proxies from stormsia", len(proxies))
	if len(proxies) == 0 {
		r.setLastError(context.Canceled)
		return
	}

	// 2. Speed test the proxies
	results := SpeedTestProxies(refreshCtx, proxies, r.config.ProbeURL, r.config.Concurrency)
	SortByLatency(results)

	alive := AliveProxies(results)
	log.Printf("[PROXY-FETCHER] speed test complete: %d/%d alive", len(alive), len(results))

	if len(alive) == 0 {
		r.setLastError(context.Canceled)
		return
	}

	// 3. Update or create the proxy pool
	poolData := ToProxyPoolData(r.config.PoolName, alive)
	if poolData == nil {
		r.setLastError(context.Canceled)
		return
	}

	// Add metadata
	poolData["source"] = "stormsia-auto"
	poolData["refreshedAt"] = time.Now().UTC().Format(time.RFC3339)
	poolData["totalTested"] = len(proxies)
	poolData["aliveCount"] = len(alive)

	// Use fixed pool ID if configured, otherwise use a stable ID
	poolID := r.config.PoolID
	if poolID == "" {
		poolID = "stormsia-residential-auto"
	}

	// Check if pool exists
	existing, err := r.repo.GetProxyPool(poolID)
	if err != nil || existing == nil {
		// Create new pool
		log.Printf("[PROXY-FETCHER] creating new proxy pool: %s", poolID)
		if err := r.createPool(poolID, poolData); err != nil {
			log.Printf("[PROXY-FETCHER] failed to create pool: %v", err)
			r.setLastError(err)
			return
		}
	} else {
		// Update existing pool
		log.Printf("[PROXY-FETCHER] updating existing proxy pool: %s", poolID)
		if err := r.updatePool(poolID, poolData); err != nil {
			log.Printf("[PROXY-FETCHER] failed to update pool: %v", err)
			r.setLastError(err)
			return
		}
	}

	// Update test status for all proxies in the pool
	for _, result := range alive {
		_ = r.repo.SetProxyPoolStatus(poolID, "passed", LatencyMs(result.Latency))
	}

	r.mu.Lock()
	r.lastRun = time.Now()
	r.lastErr = nil
	r.mu.Unlock()

	log.Printf("[PROXY-FETCHER] refresh complete: pool %s now has %d proxies", poolID, len(alive))
}

// createPool creates a new proxy pool in the database
func (r *ProxyRefresher) createPool(poolID string, data map[string]any) error {
	now := time.Now().UTC().Format(time.RFC3339)
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal pool data: %w", err)
	}

	_, err = r.repo.RawDB().Exec(
		`INSERT INTO proxyPools (id, isActive, testStatus, data, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)`,
		poolID, 1, "passed", string(dataBytes), now, now,
	)
	if err != nil {
		return fmt.Errorf("insert proxy pool: %w", err)
	}

	// Invalidate caches since we inserted directly
	// GetProxyPool will re-read from DB on next call
	return nil
}

// updatePool updates an existing proxy pool in the database
func (r *ProxyRefresher) updatePool(poolID string, data map[string]any) error {
	// Use UpdateProxyPool which handles cache invalidation
	updates := map[string]any{
		"name":      data["name"],
		"urls":      data["urls"],
		"type":      data["type"],
		"strategy":  data["strategy"],
		"isActive":  true,
		"testStatus": "passed",
	}
	if source, ok := data["source"].(string); ok {
		updates["source"] = source
	}
	if refreshedAt, ok := data["refreshedAt"].(string); ok {
		updates["refreshedAt"] = refreshedAt
	}
	if totalTested, ok := data["totalTested"].(int); ok {
		updates["totalTested"] = totalTested
	}
	if aliveCount, ok := data["aliveCount"].(int); ok {
		updates["aliveCount"] = aliveCount
	}

	return r.repo.UpdateProxyPool(poolID, updates)
}

// setLastError stores the last error from a refresh cycle
func (r *ProxyRefresher) setLastError(err error) {
	r.mu.Lock()
	r.lastErr = err
	r.lastRun = time.Now()
	r.mu.Unlock()
	if err != nil {
		log.Printf("[PROXY-FETCHER] refresh failed: %v", err)
	}
}

// LastRun returns the time of the last refresh cycle
func (r *ProxyRefresher) LastRun() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastRun
}

// LastError returns the last error from a refresh cycle
func (r *ProxyRefresher) LastError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}

// IsStarted returns whether the refresher has been started
func (r *ProxyRefresher) IsStarted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started
}

// StartProxyRefreshLoop starts the background proxy pool refresh loop.
//
// Safe to call multiple times; extra calls are no-ops. Fail-open: a failed
// refresh is logged and the next tick retries, because a proxy refresh failure
// costs availability and never costs correctness — the gateway must still serve
// traffic with whatever pools are already configured.
func StartProxyRefreshLoop(ctx context.Context, repo *db.Repo, config RefreshConfig) {
	if repo == nil || !config.Enabled {
		return
	}

	refresher := NewProxyRefresher(repo, config)
	refresher.Start(ctx)
}
