package relay

import (
	"net/http"

	"github.com/webteleport/webteleport/transport"
)

var _ Storage = (*SessionStore)(nil)

// / transport agnostic CRUD
type Storage interface {
	// Upsert
	Upsert(k string, tssn transport.Session, tstm transport.Stream, r *http.Request)
	// Read
	GetSession(k string) (transport.Session, bool)
	Records() []*Record
	RecordsHandler(w http.ResponseWriter, r *http.Request)
	// Update
	Visited(k string)
	// Remove Session
	// if session is already removed, subsequent calls will be no-op
	// NOTE: the signature is not Remove(k string) because the key doesn't
	// uniquely identify the session. If a new session is created with the same key,
	// it should not be removed by the previous call
	RemoveSession(tssn transport.Session)
	// Rand
	Allocate(r *http.Request, root string) (key string, hostnamePath string, err error)
	Negotiate(r *http.Request, root string, tssn transport.Session, tstm transport.Stream) (key string, err error)
	// Serve
	http.Handler
}

var _ Upgrader = (*WebsocketUpgrader)(nil)
var _ Upgrader = (*WebtransportUpgrader)(nil)

type Upgrader interface {
	Root() string
	IsRoot(r *http.Request) bool
	IsUpgrade(r *http.Request) bool
	Upgrade(w http.ResponseWriter, r *http.Request) (transport.Session, transport.Stream, error)
}

var _ Relayer = (*WSServer)(nil)
var _ Relayer = (*WTServer)(nil)

type Relayer interface {
	Upgrader
	Storage
	http.Handler
}
