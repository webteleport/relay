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
	sm := NewSessionManager(host)
	r := &Relay{
		WTServer: &webtransportGo.Server{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		SessionManager: sm,
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
	WTServer       *webtransportGo.Server
	SessionManager *SessionManager
	Next           http.Handler
}

func (s *Relay) IsWebtransportUpgrade(r *http.Request) (result bool) {
	origin, _, _ := strings.Cut(r.Host, ":")
	return r.URL.Query().Get("x-webtransport-upgrade") != "" && origin == s.SessionManager.HOST
}

type WebtransportUpgraderFunc func(http.ResponseWriter, *http.Request) (*webtransportGo.Session, error)

func (s *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.IsWebtransportUpgrade(r) {
		if s.Next != nil {
			s.Next.ServeHTTP(w, r)
		} else {
			s.SessionManager.ServeHTTP(w, r)
		}
		return
	}

	ssn, err := s.WTServer.Upgrade(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("upgrading failed: %s", err))
		w.WriteHeader(500)
		return
	}

	tssn := &webtransport.WebtransportSession{ssn}
	tstm, err := tssn.OpenStream(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("stm0 init failed: %s", err))
		return
	}

	s.SessionManager.AddSession(r, tssn, tstm)
}
