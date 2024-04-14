package relay

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
)

func NewWTServer(host, port string, store Storage, tlsConfig *tls.Config) *WTServer {
	u := &WebtransportUpgrader{
		root: host,
		Server: &wt.Server{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
	s := &WTServer{
		HOST:                 host,
		Storage:              store,
		WebtransportUpgrader: u,
		PostUpgrade:          store,
	}
	u.Server.H3 = http3.Server{
		Addr:            port,
		Handler:         s,
		EnableDatagrams: true,
		TLSConfig:       tlsConfig,
	}
	return s
}

func (s *WTServer) WithPostUpgrade(p http.Handler) *WTServer {
	s.PostUpgrade = p
	return s
}

type WTServer struct {
	HOST string
	Storage
	*WebtransportUpgrader
	PostUpgrade http.Handler
}

func (s *WTServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.IsUpgrade(r) {
		tssn, tstm, err := s.Upgrade(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade webtransport session failed: %s", err))
			w.WriteHeader(500)
			return
		}

		key, err := s.Negotiate(r, s.HOST, tssn, tstm)
		if err != nil {
			slog.Warn(fmt.Sprintf("negotiate webtransport session failed: %s", err))
			return
		}

		s.Add(key, tssn, tstm, r)

		return
	}

	s.PostUpgrade.ServeHTTP(w, r)
}
