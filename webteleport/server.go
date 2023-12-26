package webteleport

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
	"github.com/quic-go/webtransport-go"
	"github.com/webteleport/server/envs"
	"github.com/webteleport/server/session"
)

var EmailName = "btwiuse"
var EmailContext = "webteleport"
var EmailSuffix = "gmail.com"

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
	// read and agree to your CA's legal documents
	certmagic.DefaultACME.Agreed = true

	// provide an email address
	certmagic.DefaultACME.Email = fmt.Sprintf("%s+%s@%s", EmailName, EmailContext, EmailSuffix)

	getCertificatesOnDemand()
}

func NewServerTLSOnDemand(next http.Handler) (*webtransport.Server, error) {
	s := &webtransport.Server{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	certmagic.DefaultACME.DisableHTTPChallenge = true
	tlsConfig, err := certmagic.TLS([]string{envs.HOST})
	if err != nil {
		return nil, err
	}
	tlsConfig.NextProtos = append([]string{"h2", "http/1.1"}, tlsConfig.NextProtos...)
	s.H3 = http3.Server{
		Addr:            envs.UDP_PORT,
		Handler:         &WebTeleportServer{s, next},
		EnableDatagrams: true,
		TLSConfig:       tlsConfig,
	}
	return s, nil
}

func NewServerTLS(next http.Handler, certFile, keyFile string) *webtransport.Server {
	s := &webtransport.Server{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	GetCertificate := func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		// Always get latest localhost.crt and localhost.key
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		return &cert, nil
	}
	s.H3 = http3.Server{
		Addr:            envs.UDP_PORT,
		Handler:         &WebTeleportServer{s, next},
		EnableDatagrams: true,
		TLSConfig: &tls.Config{
			GetCertificate: GetCertificate,
		},
	}
	return s
}

func NewServer(next http.Handler) *webtransport.Server {
	s := &webtransport.Server{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	s.H3 = http3.Server{
		Addr:            envs.UDP_PORT,
		Handler:         &WebTeleportServer{s, next},
		EnableDatagrams: true,
	}
	return s
}

// WebTeleport is a HTTP/3 server that handles:
// - UFO client registration (CONNECT HOST)
// - requests over HTTP/3 (others)
type WebTeleportServer struct {
	*webtransport.Server
	Next http.Handler
}

func (s *WebTeleportServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// passthrough normal requests to next:
	// 1. simple http / websockets (Host: x.localhost)
	// 2. webtransport (Host: x.localhost:300, not yet supported by reverseproxy)
	if !IsWebTeleportRequest(r) {
		s.Next.ServeHTTP(w, r)
		return
	}
	slog.Info(fmt.Sprint("🛸", r.RemoteAddr, r.Proto, r.Method, r.Host, r.URL.Path, r.URL.RawQuery))
	// handle ufo client registration
	// Host: ufo.k0s.io:300
	ssn, err := s.Upgrade(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("upgrading failed: %s", err))
		w.WriteHeader(500)
		return
	}
	currentSession := &session.Session{
		Session: ssn,
		Values:  r.URL.Query(),
	}
	err = currentSession.InitController(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("session init failed: %s", err))
		return
	}
	candidates := ParseDomainCandidates(r.URL.Path)
	clobber := r.URL.Query().Get("clobber")
	err = session.DefaultSessionManager.Lease(currentSession, candidates, clobber)
	if err != nil {
		slog.Warn(fmt.Sprintf("leasing failed: %s", err))
		return
	}
	go session.DefaultSessionManager.Ping(currentSession)
}
