package relay

import (
	"net/http"
	"net/url"

	"github.com/webteleport/webteleport/transport"
)

var _ ISessionManagerStorage = (*SessionManager)(nil)

// / transport agnostic CRUD
type ISessionManagerStorage interface {
	// Create
	Add(k string, tssn transport.Session, tstm transport.Stream, vals url.Values)
	// Read
	Get(k string) (transport.Session, bool)
	// Update
	IncrementVisit(k string)
	// Remove
	Remove(transport.Session)
	// Rand
	Allocate(r *http.Request, root string) (key string, hostnamePath string, err error)
	Negotiate(r *http.Request, root string, tssn transport.Session, tstm transport.Stream) (key string, err error)
}

var _ Upgrader = (*SessionManager)(nil)
var _ Upgrader = (*Relay)(nil)

type Upgrader interface {
	IsIndex(r *http.Request) bool
	IsUpgrade(r *http.Request) bool
	Upgrade(w http.ResponseWriter, r *http.Request) (transport.Session, transport.Stream, error)
}

type ISessionManagerHandler interface {
	// Public
	IndexHandler(w http.ResponseWriter, r *http.Request)
	ServeHTTP(w http.ResponseWriter, r *http.Request)

	// Admin
	ApiSessionsHandler(w http.ResponseWriter, r *http.Request)

	// Proxy
	ConnectHandler(w http.ResponseWriter, r *http.Request)
}

var _ SessionManagerInterface = (*SessionManager)(nil)

type SessionManagerInterface interface {
	ISessionManagerStorage
	ISessionManagerHandler
}
