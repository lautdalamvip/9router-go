package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// StormsiaProxy represents a proxy from stormsia.github.io/proxy-list/residential
type StormsiaProxy struct {
	Protocol string  `json:"protocol"`
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	Timeout  float64 `json:"timeout"` // seconds
	Username *string `json:"username"`
	Password *string `json:"password"`
	ExitIP   string  `json:"exit_ip"`
	Country  string  `json:"country"` // ISO code
}

// ProxyURL returns the proxy URL in the format expected by http.ProxyURL
func (p *StormsiaProxy) ProxyURL() string {
	auth := ""
	if p.Username != nil && p.Password != nil && *p.Username != "" && *p.Password != "" {
		auth = fmt.Sprintf("%s:%s@", *p.Username, *p.Password)
	}
	return fmt.Sprintf("%s://%s%s:%d", p.Protocol, auth, p.Host, p.Port)
}

// ProxyType returns the proxy type for the pool ("http", "socks4", "socks5")
func (p *StormsiaProxy) ProxyType() string {
	switch p.Protocol {
	case "socks4":
		return "socks4"
	case "socks5":
		return "socks5"
	default:
		return "http"
	}
}

// StormsiaResponse represents the JSON response from stormsia.github.io
type StormsiaResponse struct {
	Proxies []StormsiaProxy `json:"proxies"`
}

// FetchStormsiaProxies fetches the proxy list from stormsia.github.io/proxy-list/residential
// and returns the top N fastest proxies based on the reported timeout field.
func FetchStormsiaProxies(ctx context.Context, topN int) ([]StormsiaProxy, error) {
	return fetchStormsiaProxiesFrom(ctx, topN, "https://stormsia.github.io/proxy-list/residential")
}

// fetchStormsiaProxiesFrom is the internal implementation that accepts a custom URL for testing.
func fetchStormsiaProxiesFrom(ctx context.Context, topN int, targetURL string) ([]StormsiaProxy, error) {
	if topN <= 0 {
		topN = 15
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "9router-go/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch stormsia proxy list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stormsia returned HTTP %d", resp.StatusCode)
	}

	// Read the full body first to handle any leading whitespace/BOM
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read stormsia response: %w", err)
	}
	bodyStr := strings.TrimSpace(string(body))
	if bodyStr == "" || bodyStr[0] != '[' {
		return nil, fmt.Errorf("stormsia returned non-JSON response: %s", truncateString(bodyStr, 100))
	}

	// The response is a JSON array directly, not an object with a "proxies" field
	var proxies []StormsiaProxy
	if err := json.Unmarshal(body, &proxies); err != nil {
		return nil, fmt.Errorf("decode stormsia response: %w", err)
	}

	// Filter out proxies with missing host/port
	var valid []StormsiaProxy
	for _, p := range proxies {
		if p.Host == "" || p.Port <= 0 || p.Port > 65535 {
			continue
		}
		// Only support http, socks4, socks5
		switch p.Protocol {
		case "http", "https", "socks4", "socks5":
			valid = append(valid, p)
		}
	}

	if len(valid) == 0 {
		return nil, fmt.Errorf("no valid proxies found in stormsia response")
	}

	// Sort by timeout (ascending) - lower timeout means faster proxy
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].Timeout < valid[j].Timeout
	})

	// Take top N
	if len(valid) > topN {
		valid = valid[:topN]
	}

	return valid, nil
}

// SpeedTestResult contains the result of a speed test for a single proxy
type SpeedTestResult struct {
	Proxy    StormsiaProxy
	Latency  time.Duration
	Error    error
	IsAlive  bool
}

// SpeedTestProxies tests a list of proxies concurrently and returns results sorted by latency
func SpeedTestProxies(ctx context.Context, proxies []StormsiaProxy, probeURL string, concurrency int) []SpeedTestResult {
	if concurrency <= 0 {
		concurrency = 5
	}
	if probeURL == "" {
		probeURL = "https://www.google.com/generate_204"
	}

	results := make([]SpeedTestResult, len(proxies))
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for i, p := range proxies {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, proxy StormsiaProxy) {
			defer wg.Done()
			defer func() { <-sem }()

			result := testSingleProxy(ctx, proxy, probeURL)
			results[idx] = result
		}(i, p)
	}

	wg.Wait()
	return results
}

// testSingleProxy tests a single proxy by making a request through it
func testSingleProxy(ctx context.Context, proxy StormsiaProxy, probeURL string) SpeedTestResult {
	proxyURLStr := proxy.ProxyURL()
	parsedURL, err := url.Parse(proxyURLStr)
	if err != nil {
		return SpeedTestResult{Proxy: proxy, Error: fmt.Errorf("invalid proxy URL: %w", err), IsAlive: false}
	}

	tr := &http.Transport{
		Proxy:             http.ProxyURL(parsedURL),
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, 5*time.Second)
		},
	}
	defer tr.CloseIdleConnections()

	client := &http.Client{
		Transport: tr,
		Timeout:  10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return SpeedTestResult{Proxy: proxy, Error: err, IsAlive: false}
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return SpeedTestResult{Proxy: proxy, Latency: latency, Error: err, IsAlive: false}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return SpeedTestResult{Proxy: proxy, Latency: latency, Error: fmt.Errorf("HTTP %d", resp.StatusCode), IsAlive: false}
	}

	return SpeedTestResult{Proxy: proxy, Latency: latency, IsAlive: true}
}

// SortByLatency sorts speed test results by latency (fastest first)
func SortByLatency(results []SpeedTestResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].IsAlive && !results[j].IsAlive {
			return true
		}
		if !results[i].IsAlive && results[j].IsAlive {
			return false
		}
		return results[i].Latency < results[j].Latency
	})
}

// AliveProxies returns only the alive proxies from speed test results
func AliveProxies(results []SpeedTestResult) []SpeedTestResult {
	var alive []SpeedTestResult
	for _, r := range results {
		if r.IsAlive {
			alive = append(alive, r)
		}
	}
	return alive
}

// ProxyURLs returns the proxy URLs from speed test results
func ProxyURLs(results []SpeedTestResult) []string {
	var urls []string
	for _, r := range results {
		if r.IsAlive {
			urls = append(urls, r.Proxy.ProxyURL())
		}
	}
	return urls
}

// ProxyTypes returns the proxy types from speed test results
func ProxyTypes(results []SpeedTestResult) []string {
	var types []string
	for _, r := range results {
		if r.IsAlive {
			types = append(types, r.Proxy.ProxyType())
		}
	}
	return types
}

// ToProxyPoolData converts speed test results to a ProxyPoolData-like map
func ToProxyPoolData(name string, results []SpeedTestResult) map[string]any {
	urls := ProxyURLs(results)
	if len(urls) == 0 {
		return nil
	}

	// Find the fastest alive proxy to determine pool type
	poolType := "http"
	for _, r := range results {
		if r.IsAlive {
			poolType = r.Proxy.ProxyType()
			break
		}
	}

	return map[string]any{
		"name":     name,
		"urls":     urls,
		"type":     poolType,
		"strategy": "round-robin",
	}
}

// ValidateProxyURL validates that a proxy URL is properly formatted
func ValidateProxyURL(proxyURL string) bool {
	if strings.TrimSpace(proxyURL) == "" {
		return false
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "http", "https", "socks4", "socks5":
		// Host must be non-empty and not just a colon
		host := parsed.Host
		if host == "" || host == ":" {
			return false
		}
		// If there's a port, host must have content before it
		if strings.HasPrefix(host, ":") {
			return false
		}
		return true
	default:
		return false
	}
}

// NormalizeProxyURL ensures the proxy URL has a proper scheme
func NormalizeProxyURL(proxyURL string) string {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return ""
	}
	if !strings.Contains(proxyURL, "://") {
		proxyURL = "http://" + proxyURL
	}
	return proxyURL
}

// LatencyMs converts a duration to milliseconds as int64
func LatencyMs(d time.Duration) int64 {
	return int64(d / time.Millisecond)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
