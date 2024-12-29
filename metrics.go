package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// TransportStats contains various transport-level statistics
type TransportStats struct {
	BytesSent            int64         `json:"bytesSent"`
	BytesReceived        int64         `json:"bytesReceived"`
	RequestCount         int64         `json:"requestCount"`
	ResponseCount        int64         `json:"responseCount"`
	FailedRequests       int64         `json:"failedRequests"`
	ActiveRequests       int64         `json:"activeRequests"`
	TotalRequestDuration time.Duration `json:"totalRequestDuration"`
	LastRequestTime      time.Time     `json:"lastRequestTime"`
	MaxRequestDuration   time.Duration `json:"maxRequestDuration"`
	MinRequestDuration   time.Duration `json:"minRequestDuration"`
}

// MetricsTransport wraps http.Transport to collect various HTTP metrics
type MetricsTransport struct {
	Transport http.RoundTripper
	Stats     TransportStats
}

type metricsReader struct {
	rc    io.ReadCloser
	count *int64
}

func (r *metricsReader) Read(p []byte) (n int, err error) {
	n, err = r.rc.Read(p)
	*r.count += int64(n)
	return
}

func (r *metricsReader) Close() error {
	return r.rc.Close()
}

// NewMetricsTransport creates a new transport that collects HTTP metrics
func NewMetricsTransport(wrapped http.RoundTripper) *MetricsTransport {
	if wrapped == nil {
		wrapped = http.DefaultTransport
	}
	return &MetricsTransport{
		Transport: wrapped,
		Stats: TransportStats{
			MinRequestDuration: 1<<63 - 1, // Initialize to max duration
		},
	}
}

// RoundTrip implements the http.RoundTripper interface
func (t *MetricsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.Stats.ActiveRequests++
	defer func() { t.Stats.ActiveRequests-- }()

	startTime := time.Now()

	// Wrap the request body if it exists
	if req.Body != nil {
		req.Body = &metricsReader{
			rc:    req.Body,
			count: &t.Stats.BytesSent,
		}
	}
	t.Stats.RequestCount++

	// Perform the request
	resp, err := t.Transport.RoundTrip(req)

	// Calculate request duration
	duration := time.Since(startTime)
	t.Stats.TotalRequestDuration += duration
	t.Stats.LastRequestTime = time.Now()

	// Update min/max request times
	if duration > t.Stats.MaxRequestDuration {
		t.Stats.MaxRequestDuration = duration
	}
	if duration < t.Stats.MinRequestDuration {
		t.Stats.MinRequestDuration = duration
	}

	if err != nil {
		t.Stats.FailedRequests++
		return nil, err
	}

	t.Stats.ResponseCount++

	// Wrap the response body
	resp.Body = &metricsReader{
		rc:    resp.Body,
		count: &t.Stats.BytesReceived,
	}

	return resp, nil
}

// GetStats returns a copy of the current transport statistics
func (t *MetricsTransport) GetStats() TransportStats {
	return t.Stats
}

// MarshalJSON implements the json.Marshaler interface
func (t *MetricsTransport) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.GetStats())
}

// MarshalJSONIndent returns an indented JSON representation
func (t *MetricsTransport) MarshalJSONIndent(prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(t.GetStats(), prefix, indent)
}
