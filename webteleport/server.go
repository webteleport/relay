package webteleport

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/webteleport/server/envs"
	"github.com/webteleport/server/session"
)

func NewServer(next http.Handler) *webtransport.Server {
	s := &webtransport.Server{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	s.H3 = http3.Server{
		Addr:            envs.PORT,
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
	slog.Info(fmt.Sprintf("🛸", r.RemoteAddr, r.Proto, r.Method, r.Host, r.URL.Path, r.URL.RawQuery))
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
