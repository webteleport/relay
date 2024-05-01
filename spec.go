package relay

import (
	"net/http"
	"net/url"

	"github.com/webteleport/transport"
)

var _ Storage = (*SessionStore)(nil)

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
	Records() []*Record

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

var _ HTTPUpgrader = (*WebsocketUpgrader)(nil)
var _ HTTPUpgrader = (*WebtransportUpgrader)(nil)

type HTTPUpgrader interface {
	Upgrader
	http.Handler
}

var _ Upgrader = (*QuicGoUpgrader)(nil)
var _ Upgrader = (*GoQuicUpgrader)(nil)
var _ Upgrader = (*TcpUpgrader)(nil)

type Upgrader interface {
	Root() string
	Upgrade() (*Request, error)
}

type Subscriber interface {
	Subscribe(upgrader Upgrader)
}

var _ Relayer = (*WSServer)(nil)
var _ Relayer = (*WTServer)(nil)

type Relayer interface {
	http.Handler
	IsUpgrade(r *http.Request) bool
	Upgrader
	Storage
}
