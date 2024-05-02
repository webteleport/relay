package relay

import (
	"net/http"

	"github.com/webteleport/webteleport/edge"
)

// transport agnostic in-memory session storage interface
type Storage interface {
	// Get Session wrapped by http.Transport
	GetRoundTripper(k string) (http.RoundTripper, bool)

	// Record Info
	RecordsHandler(w http.ResponseWriter, r *http.Request)

	// Serve HTTP
	http.Handler

	// Subscribe to Upgrader
	edge.Subscriber
}

// Relayer is a http.Handler combined with Upgrader and Storage
type Relayer interface {
	http.Handler
	IsUpgrade(r *http.Request) bool
	edge.Upgrader
	Storage
}
