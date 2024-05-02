package relay

import (
	"net/http"

	"github.com/webteleport/webteleport/edge"
)

// dispatch to other http.Handler implementations
type Dispatcher interface {
	Dispatch(r *http.Request) http.Handler

	// shortcut to Dispatch(r).ServeHTTP(w, r)
	http.Handler
}

// edge.Edge multiplexer with builtin HTTPUpgrader
type Relayer interface {
	// dispatch to HTTPUpgrader and Storage
	Dispatcher

	// builtin HTTPUpgrader
	edge.HTTPUpgrader

	// edge.Edge multiplexer
	Storage
}

// edge.Edge multiplexer
type Storage interface {
	// Dispatch to edge.Edge
	Dispatcher

	// get Session wrapped by http.Transport
	GetRoundTripper(k string) (http.RoundTripper, bool)

	// record Info
	RecordsHandler(w http.ResponseWriter, r *http.Request)

	// subscribe to incoming stream of edge.Edge
	edge.Subscriber
}
