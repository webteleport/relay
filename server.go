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
	webtransportGo "github.com/quic-go/webtransport-go"
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

func New(host, port string, tlsConfig *tls.Config) *Relay {
	store := NewSessionStore()
	r := &Relay{
		HOST:         host,
		SessionStore: store,
		WTServer: &webtransportGo.Server{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		WSServer: NewWSServer(host, store),
	}
	r.WTServer.H3 = http3.Server{
		Addr:            port,
		Handler:         r,
		EnableDatagrams: true,
		TLSConfig:       tlsConfig,
	}
	return r
}

type Relay struct {
	HOST string
	*SessionStore
	WTServer *webtransportGo.Server
	WSServer *SessionManager
	Next     http.Handler
}

type WebtransportUpgraderFunc func(http.ResponseWriter, *http.Request) (*webtransportGo.Session, error)

func (s *Relay) IsIndex(r *http.Request) (result bool) {
	origin, _, _ := strings.Cut(r.Host, ":")
	return origin == s.HOST
}

func (s *Relay) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-webtransport-upgrade") != "" && s.IsIndex(r)
}

func (s *Relay) Upgrade(w http.ResponseWriter, r *http.Request) (transport.Session, transport.Stream, error) {
	ssn, err := s.WTServer.Upgrade(w, r)
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

func (s *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.IsUpgrade(r) {
		if s.Next != nil {
			s.Next.ServeHTTP(w, r)
		} else {
			s.WSServer.ServeHTTP(w, r)
		}
		return
	}

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

	values := r.URL.Query()
	s.Add(key, tssn, tstm, values)
}
