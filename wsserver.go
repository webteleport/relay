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

func (s *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.IsUpgrade(r) {
		tssn, tstm, err := s.Upgrade(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade websocket session failed: %s", err))
			w.WriteHeader(500)
			return
		}

		key, err := s.Negotiate(r, s.HOST, tssn, tstm)
		if err != nil {
			slog.Warn(fmt.Sprintf("negotiate websocket session failed: %s", err))
			return
		}

		s.Add(key, tssn, tstm, r)

		return
	}

	isProxy := r.Header.Get("Proxy-Connection") != "" || r.Header.Get("Proxy-Authorization") != ""
	if isProxy && os.Getenv("CONNECT") != "" {
		s.ConnectHandler(w, r)
		return
	}

	// for HTTP_PROXY r.Method = GET && r.Host = google.com
	// for HTTPS_PROXY r.Method = GET && r.Host = google.com:443
	// they are currently not supported and will be handled by the 404 handler
	if s.IsRoot(r) {
		s.IndexHandler(w, r)
		return
	}

	s.Storage.ServeHTTP(w, r)
}
