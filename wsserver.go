package relay

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

func NewWSServer(host string, store Storage) *WSServer {
	return &WSServer{
		HOST:     host,
		Storage:  store,
		Upgrader: &WebsocketUpgrader{host},
		Proxy:    NewProxyHandler(),
	}
}

type WSServer struct {
	HOST string
	Storage
	Upgrader
	Proxy http.Handler
}

func (sm *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if sm.IsUpgrade(r) {
		tssn, tstm, err := sm.Upgrade(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade websocket session failed: %s", err))
			w.WriteHeader(500)
			return
		}

		key, err := sm.Negotiate(r, sm.HOST, tssn, tstm)
		if err != nil {
			slog.Warn(fmt.Sprintf("negotiate websocket session failed: %s", err))
			return
		}

		sm.Add(key, tssn, tstm, r)

		return
	}

	isProxy := r.Header.Get("Proxy-Connection") != "" || r.Header.Get("Proxy-Authorization") != ""
	if isProxy && os.Getenv("CONNECT") != "" {
		sm.ConnectHandler(w, r)
		return
	}

	// for HTTP_PROXY r.Method = GET && r.Host = google.com
	// for HTTPs_PROXY r.Method = GET && r.Host = google.com:443
	// they are currently not supported and will be handled by the 404 handler
	if sm.IsRoot(r) {
		sm.IndexHandler(w, r)
		return
	}

	sm.Storage.ServeHTTP(w, r)
}
