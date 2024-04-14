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
	r := &WTServer{
		HOST:                 host,
		Storage:              store,
		WebtransportUpgrader: u,
	}
	u.H3 = http3.Server{
		Addr:            port,
		Handler:         r,
		EnableDatagrams: true,
		TLSConfig:       tlsConfig,
	}
	return r
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

	if s.PostUpgrade != nil {
		s.PostUpgrade.ServeHTTP(w, r)
		return
	}

	s.Storage.ServeHTTP(w, r)
}
