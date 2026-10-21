package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchStormsiaProxies(t *testing.T) {
	// Mock stormsia server
	mockProxies := `[
		{"protocol":"http","host":"1.2.3.4","port":8080,"timeout":1.5,"username":null,"password":null,"exit_ip":"1.2.3.4","country":"US"},
		{"protocol":"socks5","host":"5.6.7.8","port":1080,"timeout":2.3,"username":null,"password":null,"exit_ip":"5.6.7.8","country":"DE"},
		{"protocol":"http","host":"9.10.11.12","port":3128,"timeout":0.8,"username":null,"password":null,"exit_ip":"9.10.11.12","country":"JP"},
		{"protocol":"invalid","host":"0.0.0.0","port":0,"timeout":5.0,"username":null,"password":null,"exit_ip":"0.0.0.0","country":"XX"}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockProxies))
	}))
	defer server.Close()

	ctx := context.Background()
	proxies, err := fetchStormsiaProxiesFrom(ctx, 10, server.URL)
	if err != nil {
		t.Fatalf("fetchStormsiaProxiesFrom() error = %v", err)
	}

	// Should return 3 valid proxies (invalid protocol and port 0 filtered out)
	if len(proxies) != 3 {
		t.Errorf("FetchStormsiaProxies() returned %d proxies, want 3", len(proxies))
	}

	// Check sorting by timeout (ascending)
	if proxies[0].Timeout != 0.8 {
		t.Errorf("First proxy timeout = %f, want 0.8", proxies[0].Timeout)
	}
	if proxies[1].Timeout != 1.5 {
		t.Errorf("Second proxy timeout = %f, want 1.5", proxies[1].Timeout)
	}
	if proxies[2].Timeout != 2.3 {
		t.Errorf("Third proxy timeout = %f, want 2.3", proxies[2].Timeout)
	}

	// Check proxy URLs
	if proxies[0].ProxyURL() != "http://9.10.11.12:3128" {
		t.Errorf("First proxy URL = %s, want http://9.10.11.12:3128", proxies[0].ProxyURL())
	}
	if proxies[1].ProxyURL() != "http://1.2.3.4:8080" {
		t.Errorf("Second proxy URL = %s, want http://1.2.3.4:8080", proxies[1].ProxyURL())
	}
	if proxies[2].ProxyURL() != "socks5://5.6.7.8:1080" {
		t.Errorf("Third proxy URL = %s, want socks5://5.6.7.8:1080", proxies[2].ProxyURL())
	}

	// Check proxy types
	if proxies[0].ProxyType() != "http" {
		t.Errorf("First proxy type = %s, want http", proxies[0].ProxyType())
	}
	if proxies[1].ProxyType() != "http" {
		t.Errorf("Second proxy type = %s, want http", proxies[1].ProxyType())
	}
	if proxies[2].ProxyType() != "socks5" {
		t.Errorf("Third proxy type = %s, want socks5", proxies[2].ProxyType())
	}
}

func TestFetchStormsiaProxies_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := fetchStormsiaProxiesFrom(ctx, 10, server.URL)
	if err == nil {
		t.Error("fetchStormsiaProxiesFrom() expected error for server error, got nil")
	}
}

func TestFetchStormsiaProxies_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := fetchStormsiaProxiesFrom(ctx, 10, server.URL)
	if err == nil {
		t.Error("fetchStormsiaProxiesFrom() expected error for invalid JSON, got nil")
	}
}

func TestValidateProxyURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
	}{
		{"empty", "", false},
		{"valid http", "http://proxy.example.com:8080", true},
		{"valid https", "https://proxy.example.com:8080", true},
		{"valid socks4", "socks4://proxy.example.com:1080", true},
		{"valid socks5", "socks5://proxy.example.com:1080", true},
		{"no scheme", "proxy.example.com:8080", false},
		{"invalid scheme", "ftp://proxy.example.com:8080", false},
		{"no host", "http://:8080", false},
		{"whitespace", "  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateProxyURL(tt.input); got != tt.want {
				t.Errorf("ValidateProxyURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeProxyURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"already normalized", "http://proxy.example.com:8080", "http://proxy.example.com:8080"},
		{"add scheme", "proxy.example.com:8080", "http://proxy.example.com:8080"},
		{"whitespace", "  http://proxy.example.com:8080  ", "http://proxy.example.com:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeProxyURL(tt.input); got != tt.want {
				t.Errorf("NormalizeProxyURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSortByLatency(t *testing.T) {
	results := []SpeedTestResult{
		{IsAlive: true, Latency: 100 * time.Millisecond},
		{IsAlive: false, Latency: 0},
		{IsAlive: true, Latency: 50 * time.Millisecond},
		{IsAlive: true, Latency: 150 * time.Millisecond},
	}

	SortByLatency(results)

	if !results[0].IsAlive || results[0].Latency != 50*time.Millisecond {
		t.Errorf("First result should be alive with 50ms, got alive=%v latency=%v", results[0].IsAlive, results[0].Latency)
	}
	if !results[1].IsAlive || results[1].Latency != 100*time.Millisecond {
		t.Errorf("Second result should be alive with 100ms, got alive=%v latency=%v", results[1].IsAlive, results[1].Latency)
	}
	if !results[2].IsAlive || results[2].Latency != 150*time.Millisecond {
		t.Errorf("Third result should be alive with 150ms, got alive=%v latency=%v", results[2].IsAlive, results[2].Latency)
	}
	if results[3].IsAlive {
		t.Errorf("Fourth result should be dead, got alive=%v", results[3].IsAlive)
	}
}

func TestAliveProxies(t *testing.T) {
	results := []SpeedTestResult{
		{IsAlive: true, Latency: 100 * time.Millisecond},
		{IsAlive: false, Latency: 0},
		{IsAlive: true, Latency: 50 * time.Millisecond},
	}

	alive := AliveProxies(results)
	if len(alive) != 2 {
		t.Errorf("AliveProxies() returned %d, want 2", len(alive))
	}
}

func TestToProxyPoolData(t *testing.T) {
	results := []SpeedTestResult{
		{IsAlive: true, Latency: 100 * time.Millisecond, Proxy: StormsiaProxy{Protocol: "http", Host: "1.2.3.4", Port: 8080}},
		{IsAlive: true, Latency: 50 * time.Millisecond, Proxy: StormsiaProxy{Protocol: "socks5", Host: "5.6.7.8", Port: 1080}},
	}

	data := ToProxyPoolData("Test Pool", results)
	if data == nil {
		t.Fatal("ToProxyPoolData() returned nil")
	}

	if data["name"] != "Test Pool" {
		t.Errorf("name = %v, want Test Pool", data["name"])
	}

	urls, ok := data["urls"].([]string)
	if !ok || len(urls) != 2 {
		t.Errorf("urls = %v, want 2 strings", data["urls"])
	}

		// Check pool type - should be http (first alive proxy)
	if data["type"] != "http" {
		t.Errorf("type = %v, want http", data["type"])
	}

	if data["strategy"] != "round-robin" {
		t.Errorf("strategy = %v, want round-robin", data["strategy"])
	}
}

func TestToProxyPoolData_NoAlive(t *testing.T) {
	results := []SpeedTestResult{
		{IsAlive: false, Latency: 0},
	}

	data := ToProxyPoolData("Test Pool", results)
	if data != nil {
		t.Errorf("ToProxyPoolData() = %v, want nil for no alive proxies", data)
	}
}
