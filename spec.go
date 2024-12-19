package relay

import (
	"net/http"

	"github.com/btwiuse/dispatcher"
	"github.com/btwiuse/muxr"
	"github.com/webteleport/webteleport/edge"
)

// edge.Edge multiplexer with builtin HTTPUpgrader
type Relayer interface {
	// dispatch to HTTPUpgrader and Storage
	dispatcher.Dispatcher

	// builtin HTTPUpgrader
	edge.HTTPUpgrader

	// edge.Edge multiplexer
	Storage
}

// edge.Edge multiplexer
type Storage interface {
	// Dispatch to edge.Edge
	dispatcher.Dispatcher

	// get Session wrapped by http.Transport
	GetRoundTripper(h string) (http.RoundTripper, bool)

	// record Info
	RecordsHandler(w http.ResponseWriter, r *http.Request)

	// subscribe to incoming stream of edge.Edge
	edge.Subscriber

	// apply middleware to dispatcher
	Use(middlewares ...muxr.Middleware)

	// shortcut to dispatcher
	http.Handler
}
