package spec

import (
	"net/http"
	"net/url"

	"github.com/webteleport/transport"
)

// Request is a transport agnostic request object
type Request struct {
	Session transport.Session
	Stream  transport.Stream
	Path    string
	Values  url.Values
	Header  http.Header
	RealIP  string
}

// transport agnostic in-memory session storage interface
type Storage interface {
	// Get Session wrapped by http.Transport
	GetRoundTripper(k string) (http.RoundTripper, bool)

	// Record Info
	RecordsHandler(w http.ResponseWriter, r *http.Request)

	// Serve HTTP
	http.Handler

	// Subscribe to Upgrader
	Subscriber
}

// Upgrade incoming requests via HTTP
type HTTPUpgrader interface {
	Upgrader
	http.Handler
}

// Upgrade incoming requests
type Upgrader interface {
	Root() string
	Upgrade() (*Request, error)
}

// Subscribe to incoming requests
type Subscriber interface {
	Subscribe(upgrader Upgrader)
}

// Relayer is a http.Handler combined with Upgrader and Storage
type Relayer interface {
	http.Handler
	IsUpgrade(r *http.Request) bool
	Upgrader
	Storage
}
