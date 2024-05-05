package relay

import (
	"net/http"

	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/edge"
	"github.com/webteleport/webteleport/transport/websocket"
)

var _ Relayer = (*WSServer)(nil)

func NewWSServer(host string, store Storage) *WSServer {
	hu := &websocket.Upgrader{
		HOST: host,
	}
	s := &WSServer{
		Storage:      store,
		HTTPUpgrader: hu,
	}
	go store.Subscribe(hu)
	return s
}

func (s *WSServer) WithPostUpgrade(h http.Handler) *WSServer {
	s.PostUpgrade = h
	return s
}

type WSServer struct {
	Storage
	edge.HTTPUpgrader
	PostUpgrade http.Handler
}

func (s *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Dispatch(r).ServeHTTP(w, r)
}

func (s *WSServer) Dispatch(r *http.Request) http.Handler {
	switch {
	case s.IsUpgrade(r):
		return s.HTTPUpgrader
	case IsConnect(r):
		return ConnectHandler
	case s.IsRoot(r):
		return http.HandlerFunc(s.IndexHandler)
	case s.PostUpgrade != nil:
		return s.PostUpgrade
	default:
		return s.Storage
	}
}

func (s *WSServer) IsRoot(r *http.Request) bool {
	return utils.StripPort(r.Host) == utils.StripPort(s.HTTPUpgrader.Root())
}

func (s *WSServer) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-websocket-upgrade") != "" && s.IsRoot(r)
}
