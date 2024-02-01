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
	"github.com/quic-go/webtransport-go"
	"github.com/webteleport/relay/manager"
	"github.com/webteleport/relay/session"
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
	manager.DefaultSessionManager.HOST = host
	s := &webtransport.Server{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	r := &Relay{
		Server: s,
		Next:   next,
		HOST:   host,
	}
	s.H3 = http3.Server{
		Addr:            port,
		Handler:         r,
		EnableDatagrams: true,
		TLSConfig:       tlsConfig,
	}
	return r
}

// WebTeleport is a HTTP/3 server that handles:
// - UFO client registration (CONNECT HOST)
// - requests over HTTP/3 (others)
type Relay struct {
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
func (s *Relay) IsWebtransportUpgrade(r *http.Request) (result bool) {
	if r.URL.Query().Get("x-webtransport-upgrade") != "" {
		return true
	}
	// TODO remove in the future
	if r.URL.Query().Get("x-webteleport-upgrade") != "" {
		return true
	}
	var (
		origin, _, _ = strings.Cut(r.Host, ":")

		isWebtransport = r.Proto == "webtransport"
		isConnect      = r.Method == http.MethodConnect
		isOrigin       = origin == s.HOST
	)
	result = isWebtransport && isConnect && isOrigin
	return
}

func (s *Relay) IsWebsocketUpgrade(r *http.Request) (result bool) {
	if r.URL.Query().Get("x-websocket-upgrade") != "" {
		return true
	}
	var (
		origin, _, _ = strings.Cut(r.Host, ":")

		isWebsocket = (r.Header.Get("Connection") == "upgrade" && r.Header.Get("Upgrade") == "websocket")
		isGet       = r.Method == http.MethodGet
		isOrigin    = origin == s.HOST
	)
	result = isWebsocket && isGet && isOrigin
	return
}

func (s *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// passthrough normal requests to next:
	// 1. simple http / websockets (Host: x.localhost)
	// 2. webtransport (Host: x.localhost:300, not yet supported by reverseproxy)

	println(r.URL.String())
	var currentSession manager.Session
	switch {
	case s.IsWebtransportUpgrade(r):
		slog.Info(fmt.Sprint("🛸 webtransport", r.RemoteAddr, r.Proto, r.Method, r.Host, r.URL.Path, r.URL.RawQuery))

		// handle ufo client registration
		// Host: ufo.k0s.io:300
		ssn, err := s.Upgrade(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrading failed: %s", err))
			w.WriteHeader(500)
			return
		}
		currentSession = &session.WebtransportSession{
			Session: ssn,
			Values:  r.URL.Query(),
		}
		err = currentSession.InitController(context.Background())
		if err != nil {
			slog.Warn(fmt.Sprintf("session init failed: %s", err))
			return
		}
		manager.AddManagerSession(currentSession, r)
	case s.IsWebsocketUpgrade(r): // this case will never be reached, since it's handled in DefaultSessionManager
		slog.Info(fmt.Sprint("🛸 websocket", r.RemoteAddr, r.Proto, r.Method, r.Host, r.URL.Path, r.URL.RawQuery))
		// handle ufo client registration
		// Host: ufo.k0s.io:300
		currentSession, err := manager.UpgradeWebsocketSession(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade websocket session failed: %s", err))
			return
		}
		manager.AddManagerSession(currentSession, r)
	default:
		s.Next.ServeHTTP(w, r)
	}
}

/*
func AddManagerSession(currentSession manager.Session, r *http.Request) {
	// common logic
	var (
		candidates = utils.ParseDomainCandidates(r.URL.Path)
		clobber    = r.URL.Query().Get("clobber")
	)

	if err := manager.DefaultSessionManager.Lease(currentSession, candidates, clobber); err != nil {
		slog.Warn(fmt.Sprintf("leasing failed: %s", err))
		return
	}
	go manager.DefaultSessionManager.Ping(currentSession)
	go manager.DefaultSessionManager.Scan(currentSession)
}

func UpgradeWebsocketSession(w http.ResponseWriter, r *http.Request) (manager.Session, error) {
	conn, err := wrap.Wrconn(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("upgrading failed: %s", err))
		w.WriteHeader(500)
		return nil, err
	}
	ssn, err := yamux.Server(conn, nil)
	if err != nil {
		slog.Warn(fmt.Sprintf("creating yamux.Server failed: %s", err))
		w.WriteHeader(500)
		return nil, err
	}
	managerSession := &session.WebsocketSession{
		Session: ssn,
		Values:  r.URL.Query(),
	}
	err = managerSession.InitController(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("session init failed: %s", err))
		return nil, err
	}
	return managerSession, nil
}
*/
