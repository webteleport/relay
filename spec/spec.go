package spec

import (
	"net/http"
	"net/url"

	"github.com/webteleport/transport"
)

type Request struct {
	Session transport.Session
	Stream  transport.Stream
	Path    string
	Values  url.Values
	Header  http.Header
	RealIP  string
}

// / transport agnostic in-memory session storage
type Storage interface {
	// Read
	// TODO: make it private
	GetSession(k string) (transport.Session, bool)
	// Records() []*Record
	RecordsHandler(w http.ResponseWriter, r *http.Request)

	// Update
	// TODO: make it private
	Visited(k string)

	/* internal */
	// Upsert
	/// Upsert(k string, r *Request)
	// Remove Session
	// if session is already removed, subsequent calls will be no-op
	// NOTE: the signature is not Remove(k string) because the key doesn't
	// uniquely identify the session. If a new session is created with the same key,
	// it should not be removed by the previous call
	/// RemoveSession(tssn transport.Session)
	// Rand
	/// Allocate(r *Request, root string) (key string, hostnamePath string, err error)
	/// Negotiate(r *Request, root string) (key string, err error)

	// Serve HTTP
	http.Handler

	// Subscribe
	Subscriber
}

type HTTPUpgrader interface {
	Upgrader
	http.Handler
}

type Upgrader interface {
	Root() string
	Upgrade() (*Request, error)
}

type Subscriber interface {
	Subscribe(upgrader Upgrader)
}

type Relayer interface {
	http.Handler
	IsUpgrade(r *http.Request) bool
	Upgrader
	Storage
}
