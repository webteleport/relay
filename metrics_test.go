package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestMetricsTransportConcurrency verifies that concurrent RoundTrip calls
// do not trigger data races on TransportStats fields.
func TestMetricsTransportConcurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	mt := NewMetricsTransport(http.DefaultTransport)

	const goroutines = 50
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range requestsPerGoroutine {
				body := strings.NewReader(fmt.Sprintf("body-%d-%d", id, j))
				req, err := http.NewRequest("POST", server.URL, body)
				if err != nil {
					t.Errorf("failed to create request: %v", err)
					return
				}
				resp, err := mt.RoundTrip(req)
				if err != nil {
					t.Errorf("RoundTrip failed: %v", err)
					return
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}(i)
	}
	wg.Wait()

	// Verify final counter values
	totalRequests := atomic.LoadInt64(&mt.Stats.RequestCount)
	totalResponses := atomic.LoadInt64(&mt.Stats.ResponseCount)
	activeRequests := atomic.LoadInt64(&mt.Stats.ActiveRequests)

	expected := int64(goroutines * requestsPerGoroutine)
	if totalRequests != expected {
		t.Errorf("RequestCount = %d, want %d", totalRequests, expected)
	}
	if totalResponses != expected {
		t.Errorf("ResponseCount = %d, want %d", totalResponses, expected)
	}
	if activeRequests != 0 {
		t.Errorf("ActiveRequests = %d, want 0", activeRequests)
	}
}

// TestMetricsTransportMarshalJSON verifies that JSON marshaling produces
// a valid snapshot while concurrent requests are in flight.
func TestMetricsTransportMarshalJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("response"))
	}))
	defer server.Close()

	mt := NewMetricsTransport(http.DefaultTransport)

	// Fire off some concurrent requests and marshal JSON simultaneously
	var wg sync.WaitGroup

	// Goroutines making requests
	wg.Add(10)
	for range 10 {
		go func() {
			defer wg.Done()
			for range 5 {
				req, _ := http.NewRequest("GET", server.URL, nil)
				resp, err := mt.RoundTrip(req)
				if err == nil {
					io.ReadAll(resp.Body)
					resp.Body.Close()
				}
			}
		}()
	}

	// Goroutines marshaling JSON concurrently
	wg.Add(5)
	for range 5 {
		go func() {
			defer wg.Done()
			for range 10 {
				data, err := mt.MarshalJSON()
				if err != nil {
					t.Errorf("MarshalJSON failed: %v", err)
					return
				}
				var stats TransportStats
				if err := json.Unmarshal(data, &stats); err != nil {
					t.Errorf("Unmarshal failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// TestMetricsTransportFailedRequests verifies that failed requests are counted.
func TestMetricsTransportFailedRequests(t *testing.T) {
	// Transport that always fails
	failingTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("connection refused")
	})

	mt := NewMetricsTransport(failingTransport)

	req, _ := http.NewRequest("GET", "http://localhost:1", nil)
	_, err := mt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error from failing transport")
	}

	if atomic.LoadInt64(&mt.Stats.FailedRequests) != 1 {
		t.Errorf("FailedRequests = %d, want 1", mt.Stats.FailedRequests)
	}
	if atomic.LoadInt64(&mt.Stats.RequestCount) != 1 {
		t.Errorf("RequestCount = %d, want 1", mt.Stats.RequestCount)
	}
	if atomic.LoadInt64(&mt.Stats.ResponseCount) != 0 {
		t.Errorf("ResponseCount = %d, want 0", mt.Stats.ResponseCount)
	}
}

// TestMetricsTransportDurationTracking verifies min/max/total duration tracking.
func TestMetricsTransportDurationTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	mt := NewMetricsTransport(http.DefaultTransport)

	for range 3 {
		req, _ := http.NewRequest("GET", server.URL, nil)
		resp, err := mt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip failed: %v", err)
		}
		resp.Body.Close()
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	if mt.Stats.TotalRequestDuration <= 0 {
		t.Error("TotalRequestDuration should be positive")
	}
	if mt.Stats.MaxRequestDuration <= 0 {
		t.Error("MaxRequestDuration should be positive")
	}
	if mt.Stats.MinRequestDuration <= 0 {
		t.Error("MinRequestDuration should be positive")
	}
	if mt.Stats.MinRequestDuration > mt.Stats.MaxRequestDuration {
		t.Error("MinRequestDuration should be <= MaxRequestDuration")
	}
	if mt.Stats.LastRequestTime.IsZero() {
		t.Error("LastRequestTime should not be zero")
	}
}

// roundTripFunc is a helper to use a function as http.RoundTripper
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
