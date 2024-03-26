package relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"

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

func New(host, port string, next http.Handler, tlsConfig *tls.Config) *Relay {
	sessionManager := NewSessionManager(host)
	s := &webtransportGo.Server{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	r := &Relay{
		Server:         s,
		SessionManager: sessionManager,
		Next:           next,
	}
	s.H3 = http3.Server{
		Addr:            port,
		Handler:         r.UpgradeWebtransportHandler(),
		EnableDatagrams: true,
		TLSConfig:       tlsConfig,
	}
	return r
}

type Relay struct {
	*webtransportGo.Server
	SessionManager *SessionManager
	Next           http.Handler
}

func IsWebtransportUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-webtransport-upgrade") != ""
}

func (s *Relay) UpgradeWebtransportHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsWebtransportUpgrade(r) {
			s.Next.ServeHTTP(w, r)
		}

		ssn, err := s.Upgrade(w, r)
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
	})
}
