package relay

import (
	"net/http"

	"github.com/webteleport/relay/spec"
)

var _ spec.Relayer = (*WSServer)(nil)

func NewWSServer(host string, store spec.Storage) *WSServer {
	hu := &WebsocketUpgrader{
		HOST: host,
	}
	s := &WSServer{
		Storage:      store,
		HTTPUpgrader: hu,
		Connect:      NewConnectHandler(),
	}
	go store.Subscribe(hu)
	return s
}

func (s *WSServer) WithPostUpgrade(h http.Handler) *WSServer {
	s.PostUpgrade = h
	return s
}

type WSServer struct {
	spec.Storage
	spec.HTTPUpgrader
	Connect     http.Handler
	PostUpgrade http.Handler
}

func (s *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.IsUpgrade(r) {
		s.HTTPUpgrader.ServeHTTP(w, r)
		return
	}

	if IsConnect(r) {
		s.ConnectHandler(w, r)
		return
	}

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

func (s *WSServer) IsRoot(r *http.Request) bool {
	return r.Host == s.HTTPUpgrader.Root()
}

func (s *WSServer) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-websocket-upgrade") != "" && s.IsRoot(r)
}
