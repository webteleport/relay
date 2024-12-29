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

// metricReader wraps an io.Reader to count bytes read
type metricReader struct {
	r     io.Reader
	count *int64
}

func (r *metricReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	*r.count += int64(n)
	return
}

// metricWriter wraps an io.Writer to count bytes written
type metricWriter struct {
	w     io.Writer
	count *int64
}

func (w *metricWriter) Write(p []byte) (n int, err error) {
	n, err = w.w.Write(p)
	*w.count += int64(n)
	return
}

// metricsReadCloser implements io.ReadCloser with metrics
type metricsReadCloser struct {
	metricReader
	closer io.Closer
}

func (r *metricsReadCloser) Close() error {
	return r.closer.Close()
}

// metricsReadWriteCloser implements io.ReadWriteCloser with metrics
type metricsReadWriteCloser struct {
	metricReader
	metricWriter
	closer io.Closer
}

func (rw *metricsReadWriteCloser) Close() error {
	return rw.closer.Close()
}

// wrapBody wraps a body with metrics tracking, handling both ReadCloser and ReadWriteCloser cases
func wrapBody(body io.ReadCloser, readCount, writeCount *int64) io.ReadCloser {
	if rwc, ok := body.(io.ReadWriteCloser); ok {
		return &metricsReadWriteCloser{
			metricReader: metricReader{r: rwc, count: readCount},
			metricWriter: metricWriter{w: rwc, count: writeCount},
			closer:       rwc,
		}
	}
	return &metricsReadCloser{
		metricReader: metricReader{r: body, count: readCount},
		closer:       body,
	}
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
		req.Body = wrapBody(req.Body, &t.Stats.BytesReceived, &t.Stats.BytesSent)
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
	resp.Body = wrapBody(resp.Body, &t.Stats.BytesReceived, &t.Stats.BytesSent)

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
