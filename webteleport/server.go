package webteleport

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
	"github.com/quic-go/webtransport-go"
	"github.com/webteleport/relay/session"
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

func NewServerTLS(host, port string, next http.Handler, tlsConfig *tls.Config) *webtransport.Server {
	session.DefaultSessionManager.HOST = host
	s := &webtransport.Server{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	wts := &WebTeleportServer{
		Server: s,
		Next:   next,
		HOST:   host,
	}
	s.H3 = http3.Server{
		Addr:            port,
		Handler:         wts,
		EnableDatagrams: true,
		TLSConfig:       tlsConfig,
	}
	return s
}

// WebTeleport is a HTTP/3 server that handles:
// - UFO client registration (CONNECT HOST)
// - requests over HTTP/3 (others)
type WebTeleportServer struct {
	*webtransport.Server
	Next http.Handler
	HOST string
}

// IsWebTeleportRequest tells if the incoming request should be treated as UFO request
//
// An UFO request must meet all criteria:
//
// - r.Proto == "webtransport"
// - r.Method == "CONNECT"
// - origin (r.Host without port) matches HOST
//
// if all true, it will be upgraded into a webtransport session
// otherwise the request will be handled by DefaultSessionManager
func (s *WebTeleportServer) IsWebTeleportRequest(r *http.Request) bool {
	var (
		origin, _, _ = strings.Cut(r.Host, ":")

		isWebtransport = r.Proto == "webtransport"
		isConnect      = r.Method == http.MethodConnect
		isOrigin       = origin == s.HOST

		isWebTeleport = isWebtransport && isConnect && isOrigin
	)
	return isWebTeleport
}

func (s *WebTeleportServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// passthrough normal requests to next:
	// 1. simple http / websockets (Host: x.localhost)
	// 2. webtransport (Host: x.localhost:300, not yet supported by reverseproxy)
	if !s.IsWebTeleportRequest(r) {
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
