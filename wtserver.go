package relay

import (
	"crypto/tls"
	"net/http"

	"github.com/btwiuse/dispatcher"
	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/webtransport"
)

var _ Relayer = (*WTServer)(nil)

func DefaultWTServer(host string) *WTServer {
	return NewWTServer(host, DefaultIngress)
}

func NewWTServer(host string, ingress Ingress) *WTServer {
	hu := &webtransport.Upgrader{
		Server: &wt.Server{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		RootPatterns: []string{host},
	}
	s := &WTServer{
		Ingress:  ingress,
		Upgrader: hu,
	}
	hu.Server.H3 = http3.Server{
		Handler: s,
		// WebTransport requires DATAGRAM support
		EnableDatagrams: true,
	}
	go ingress.Subscribe(hu)
	return s
}

func (s *WTServer) WithAddr(a string) *WTServer {
	s.Upgrader.Server.H3.Addr = a
	return s
}

func (s *WTServer) WithTLSConfig(tlsConfig *tls.Config) *WTServer {
	s.Upgrader.Server.H3.TLSConfig = tlsConfig
	return s
}

type WTServer struct {
	Ingress
	*webtransport.Upgrader
}

func (s *WTServer) Dispatch(r *http.Request) http.Handler {
	switch {
	case s.IsUpgrade(r):
		return s.Upgrader
	case IsProxy(r):
		return AuthenticatedProxyHandler
	default:
		return s.Ingress
	}
}

func (s *WTServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatcher.DispatcherFunc(s.Dispatch).ServeHTTP(w, r)
}

func (s *WTServer) IsRootExternal(r *http.Request) bool {
	return s.Upgrader.IsRoot(utils.StripPort(r.Host))
}

func (s *WTServer) IsRootInternal(r *http.Request) bool {
	return utils.StripPort(r.Host) == ROOT_INTERNAL
}

func (s *WTServer) IsUpgrade(r *http.Request) bool {
	isHeader := r.Header.Get(webtransport.UpgradeHeader) != ""
	isQuery := r.URL.Query().Get(webtransport.UpgradeQuery) != ""
	isRoot := s.IsRootExternal(r)
	return isRoot && (isHeader || isQuery)
}
