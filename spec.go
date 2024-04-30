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

// / transport agnostic CRUD
type Storage interface {
	// Upsert
	Upsert(k string, r *Request)
	// Read
	GetSession(k string) (transport.Session, bool)
	Records() []*Record
	// Update
	Visited(k string)
	// Remove Session
	// if session is already removed, subsequent calls will be no-op
	// NOTE: the signature is not Remove(k string) because the key doesn't
	// uniquely identify the session. If a new session is created with the same key,
	// it should not be removed by the previous call
	RemoveSession(tssn transport.Session)
	// Rand
	Allocate(r *Request, root string) (key string, hostnamePath string, err error)
	Negotiate(r *Request, root string) (key string, err error)
	// Serve HTTP
	http.Handler
}

var _ HTTPUpgrader = (*WebsocketUpgrader)(nil)
var _ HTTPUpgrader = (*WebtransportUpgrader)(nil)

type HTTPUpgrader interface {
	Root() string
	IsRoot(r *http.Request) bool
	IsUpgrade(r *http.Request) bool
	Upgrade(w http.ResponseWriter, r *http.Request) (*Request, error)
}

var _ Upgrader = (*QuicGoUpgrader)(nil)
var _ Upgrader = (*GoQuicUpgrader)(nil)
var _ Upgrader = (*TcpUpgrader)(nil)

type Upgrader interface {
	Host() string
	Upgrade() (*Request, error)
}

var _ Relayer = (*WSServer)(nil)
var _ Relayer = (*WTServer)(nil)

type Relayer interface {
	Storage
	http.Handler
}
