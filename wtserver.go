package relay

import (
	"crypto/tls"
	"net/http"

	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/webtransport"
)

var _ Relayer = (*WTServer)(nil)

func NewWTServer(host string, store Storage) *WTServer {
	hu := &webtransport.Upgrader{
		HOST: host,
		Server: &wt.Server{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
	s := &WTServer{
		HOST:     host,
		Storage:  store,
		Upgrader: hu,
	}
	hu.Server.H3 = http3.Server{
		Handler: s,
		// WebTransport requires DATAGRAM support
		EnableDatagrams: true,
	}
	go store.Subscribe(hu)
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
	HOST string
	Storage
	*webtransport.Upgrader
}

func (s *WTServer) Dispatch(r *http.Request) http.Handler {
	switch {
	case s.IsUpgrade(r):
		return s.Upgrader
	case IsInternal(r):
		return http.HandlerFunc(handleInternal)
	case IsProxy(r):
		return AuthenticatedProxyHandler
	default:
		return s.Storage
	}
}

func (s *WTServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	DispatcherFunc(s.Dispatch).ServeHTTP(w, r)
}

func (s *WTServer) IsRoot(r *http.Request) bool {
	return utils.StripPort(r.Host) == utils.StripPort(s.HOST)
}

func (s *WTServer) IsUpgrade(r *http.Request) bool {
	isHeader := r.Header.Get(webtransport.UpgradeHeader) != ""
	isQuery := r.URL.Query().Get(webtransport.UpgradeQuery) != ""
	isRoot := s.IsRoot(r)
	return isRoot && (isHeader || isQuery)
}
