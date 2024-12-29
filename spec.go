package relay

import (
	"net/http"

	"github.com/btwiuse/dispatcher"
	"github.com/btwiuse/muxr"
	"github.com/webteleport/webteleport/edge"
	"github.com/webteleport/webteleport/tunnel"
)

// edge.Edge multiplexer with builtin HTTPUpgrader
type Relayer interface {
	// dispatch to HTTPUpgrader and Storage
	dispatcher.Dispatcher

	// builtin HTTPUpgrader
	edge.HTTPUpgrader

	// edge.Edge consumer
	Ingress
}

// storage exposed as HTTP server
type Ingress interface {
	// shortcut to dispatcher
	http.Handler

	// apply middleware to dispatcher
	Use(middlewares ...muxr.Middleware)

	// get Session wrapped by http.Transport
	GetRoundTripper(h string) (http.RoundTripper, bool)

	// alias Info
	AliasHandler(w http.ResponseWriter, r *http.Request)

	// record Info
	RecordsHandler(w http.ResponseWriter, r *http.Request)

	// subscribe to incoming stream of edge.Edge
	edge.Subscriber
}

// edge.Edge multiplexer
type Storage interface {
	// allocate new session
	Allocate(r *edge.Edge) (string, error)

	// remove session
	RemoveSession(tssn tunnel.Session)

	// upsert session
	Upsert(k string, r *edge.Edge)

	// get record
	GetRecord(h string) (*Record, bool)

	// get all records
	Records() (all []*Record)

	// scan edge.Edge
	Scan(r *edge.Edge)

	// ping edge.Edge
	Ping(r *edge.Edge)

	// subscribe to incoming stream of edge.Edge
	edge.Subscriber

	// log message
	WebLog(msg string)

	// alias
	Alias(k string, v string)

	// unalias
	Unalias(k string)

	// get all aliases
	Aliases() (all map[string]string)

	// lookup record
	LookupRecord(k string) (rec *Record, ok bool)
}
