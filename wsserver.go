package relay

import (
	"net/http"

	"github.com/btwiuse/dispatcher"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/edge"
	"github.com/webteleport/webteleport/transport/websocket"
)

var _ Relayer = (*WSServer)(nil)

func DefaultWSServer(host string) *WSServer {
	return NewWSServer(host, DefaultIngress)
}

func NewWSServer(host string, ingress Ingress) *WSServer {
	hu := &websocket.Upgrader{
		RootPatterns: []string{host},
	}
	s := &WSServer{
		Ingress:      ingress,
		HTTPUpgrader: hu,
	}
	go ingress.Subscribe(hu)
	return s
}

type WSServer struct {
	Ingress
	edge.HTTPUpgrader
}

func (s *WSServer) Dispatch(r *http.Request) http.Handler {
	switch {
	case s.IsUpgrade(r):
		return s.HTTPUpgrader
	case s.IsRootInternal(r):
		return http.HandlerFunc(s.RootInternalHandler)
	case IsInternal(r):
		return http.HandlerFunc(handleInternal)
	case s.IsRootExternal(r):
		return http.HandlerFunc(s.RootHandler)
	case IsProxy(r):
		return AuthenticatedProxyHandler
	default:
		return s.Ingress
	}
}

func (s *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatcher.DispatcherFunc(s.Dispatch).ServeHTTP(w, r)
}

func (s *WSServer) IsRootExternal(r *http.Request) bool {
	return s.HTTPUpgrader.IsRoot(utils.StripPort(r.Host))
}

func (s *WSServer) IsRootInternal(r *http.Request) bool {
	return utils.StripPort(r.Host) == ROOT_INTERNAL
}

func (s *WSServer) IsUpgrade(r *http.Request) (result bool) {
	isHeader := r.Header.Get(websocket.UpgradeHeader) != ""
	isQuery := r.URL.Query().Get(websocket.UpgradeQuery) != ""
	isRoot := s.IsRootExternal(r)
	return isRoot && (isHeader || isQuery)
}
