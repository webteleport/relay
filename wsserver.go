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

type WSServer struct {
	Storage
	edge.HTTPUpgrader
}

func (s *WSServer) Dispatch(r *http.Request) http.Handler {
	switch {
	case s.IsUpgrade(r):
		return s.HTTPUpgrader
	case IsAuthenticatedProxy(r):
		return AuthenticatedProxyHandler
	case s.IsRoot(r):
		return http.HandlerFunc(s.IndexHandler)
	default:
		return s.Storage
	}
}

func (s *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	DispatcherFunc(s.Dispatch).ServeHTTP(w, r)
}

func (s *WSServer) IsRoot(r *http.Request) bool {
	return utils.StripPort(r.Host) == utils.StripPort(s.HTTPUpgrader.Root())
}

func (s *WSServer) IsUpgrade(r *http.Request) (result bool) {
	isHeader := r.Header.Get(websocket.UpgradeHeader) != ""
	isQuery := r.URL.Query().Get(websocket.UpgradeQuery) != ""
	isRoot := s.IsRoot(r)
	return isRoot && (isHeader || isQuery)
}
