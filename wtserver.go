package relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/digitalocean"
	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
	"github.com/webteleport/webteleport/transport"
	"github.com/webteleport/webteleport/transport/webtransport"
)

func getCertificatesOnDemand() {
	// if the decision function returns an error, a certificate
	// may not be obtained for that name at that time
	certmagic.Default.OnDemand = &certmagic.OnDemandConfig{
		DecisionFunc: func(_ctx context.Context, name string) error {
			return nil
		},
	}
}

func getWildcardCertificates() {
	certmagic.DefaultACME.DNS01Solver = &certmagic.DNS01Solver{
		DNSProvider: &digitalocean.Provider{
			APIToken: os.Getenv("DIGITALOCEAN_ACCESS_TOKEN"),
		},
	}
}

func init() {
	var EmailName = "btwiuse"
	var EmailContext = "webteleport"
	var EmailSuffix = "gmail.com"

	// provide an email address
	certmagic.DefaultACME.Email = fmt.Sprintf("%s+%s@%s", EmailName, EmailContext, EmailSuffix)

	// read and agree to your CA's legal documents
	certmagic.DefaultACME.Agreed = true

	getCertificatesOnDemand()
}

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

type WebtransportUpgrader struct {
	root string
	*wt.Server
}

func (s *WebtransportUpgrader) Root() string {
	return s.root
}

func (s *WebtransportUpgrader) IsRoot(r *http.Request) (result bool) {
	origin, _, _ := strings.Cut(r.Host, ":")
	return origin == s.Root()
}

func (s *WebtransportUpgrader) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-webtransport-upgrade") != "" && s.IsRoot(r)
}

func (s *WebtransportUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (transport.Session, transport.Stream, error) {
	ssn, err := s.Server.Upgrade(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("upgrading failed: %s", err))
		w.WriteHeader(500)
		return nil, nil, err
	}

	tssn := &webtransport.WebtransportSession{ssn}
	tstm, err := tssn.OpenStream(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("stm0 init failed: %s", err))
		return nil, nil, err
	}

	return tssn, tstm, nil
}