package relay

import (
	"crypto/tls"
	"net/http"

	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
	"github.com/webteleport/webteleport/spec"
	"github.com/webteleport/utils"
)

var _ Relayer = (*WTServer)(nil)

func NewWTServer(host string, store Storage) *WTServer {
	hu := &WebtransportUpgrader{
		root: host,
		reqc: make(chan *spec.Request, 10),
		Server: &wt.Server{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
	s := &WTServer{
		HOST:                 host,
		Storage:              store,
		WebtransportUpgrader: hu,
		Connect:              NewConnectHandler(),
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
	s.WebtransportUpgrader.Server.H3.Addr = a
	return s
}

func (s *WTServer) WithTLSConfig(tlsConfig *tls.Config) *WTServer {
	s.WebtransportUpgrader.Server.H3.TLSConfig = tlsConfig
	return s
}

func (s *WTServer) WithPostUpgrade(h http.Handler) *WTServer {
	s.PostUpgrade = h
	return s
}

type WTServer struct {
	HOST string
	Storage
	*WebtransportUpgrader
	Connect     http.Handler
	PostUpgrade http.Handler
}

func (s *WTServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.IsUpgrade(r) {
		s.WebtransportUpgrader.ServeHTTP(w, r)
		return
	}

	if IsConnect(r) {
		s.ConnectHandler(w, r)
		return
	}

	if s.PostUpgrade != nil {
		s.PostUpgrade.ServeHTTP(w, r)
		return
	}

	s.Storage.ServeHTTP(w, r)
}

func (s *WTServer) IsRoot(r *http.Request) bool {
	return utils.StripPort(r.Host) == utils.StripPort(s.HOST)
}

func (s *WTServer) IsUpgrade(r *http.Request) bool {
	return r.URL.Query().Get("x-webtransport-upgrade") != "" && s.IsRoot(r)
}
