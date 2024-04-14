package relay

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
)

func NewWTServer(host string, store Storage) *WTServer {
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
	}
	u.Server.H3 = http3.Server{
		Handler: s,
	}
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

func (s *WTServer) WithHandler(h http.Handler) *WTServer {
	s.WebtransportUpgrader.Server.H3.Handler = h
	return s
}

type WTServer struct {
	HOST string
	Storage
	*WebtransportUpgrader
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

	s.Storage.ServeHTTP(w, r)
}
