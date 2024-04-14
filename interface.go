package relay

import (
	"net/http"
	"net/url"

	"github.com/webteleport/webteleport/transport"
)

var _ Storage = (*SessionStore)(nil)

// / transport agnostic CRUD
type Storage interface {
	// Create
	Add(k string, tssn transport.Session, tstm transport.Stream, vals url.Values)
	// Read
	Get(k string) (transport.Session, bool)
	Records() []*Record
	RecordsHandler(w http.ResponseWriter, r *http.Request)
	// Update
	Visited(k string)
	// Remove
	Remove(k string)
	// Rand
	Allocate(r *http.Request, root string) (key string, hostnamePath string, err error)
	Negotiate(r *http.Request, root string, tssn transport.Session, tstm transport.Stream) (key string, err error)
}

var _ Upgrader = (*Relay)(nil)
var _ Upgrader = (*WSServer)(nil)

type Upgrader interface {
	IsIndex(r *http.Request) bool
	IsUpgrade(r *http.Request) bool
	Upgrade(w http.ResponseWriter, r *http.Request) (transport.Session, transport.Stream, error)
}

var _ IRelay = (*Relay)(nil)
var _ IRelay = (*WSServer)(nil)

type IRelay interface {
	Upgrader
	Storage
	http.Handler
}
