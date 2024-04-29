package relay

import (
	"fmt"
	"log/slog"
	"net/http"
)

func NewWSServer(host string, store Storage) *WSServer {
	return &WSServer{
		HOST:         host,
		Storage:      store,
		HTTPUpgrader: &WebsocketUpgrader{host},
		Connect:      NewConnectHandler(),
	}
}

func (s *WSServer) WithPostUpgrade(h http.Handler) *WSServer {
	s.PostUpgrade = h
	return s
}

type WSServer struct {
	HOST string
	Storage
	HTTPUpgrader
	Connect     http.Handler
	PostUpgrade http.Handler
}

func (s *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.IsUpgrade(r) {
		R, err := s.Upgrade(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade websocket session failed: %s", err))
			w.WriteHeader(500)
			return
		}

		key, err := s.Negotiate(R, s.HOST)
		if err != nil {
			slog.Warn(fmt.Sprintf("negotiate websocket session failed: %s", err))
			return
		}

		s.Upsert(key, R)

		return
	}

	if IsConnect(r) {
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

	if s.PostUpgrade != nil {
		s.PostUpgrade.ServeHTTP(w, r)
		return
	}

	s.Storage.ServeHTTP(w, r)
}
