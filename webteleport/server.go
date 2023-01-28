package webteleport

import (
	"context"
	"log"
	"net/http"

	"github.com/webtransport/quic-go/http3"
	"github.com/webtransport/webtransport-go"
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
	log.Println("🛸", r.RemoteAddr, r.Proto, r.Method, r.Host, r.URL.Path)
	// handle ufo client registration
	// Host: ufo.k0s.io:300
	ssn, err := s.Upgrade(w, r)
	if err != nil {
		log.Printf("upgrading failed: %s", err)
		w.WriteHeader(500)
		return
	}
	currentSession := &session.Session{
		Session: ssn,
	}
	err = currentSession.InitController(context.Background())
	if err != nil {
		log.Printf("session init failed: %s", err)
		return
	}
	candidates := ParseDomainCandidates(r.URL.Path)
	err = session.DefaultSessionManager.Lease(currentSession, candidates)
	if err != nil {
		log.Printf("leasing failed: %s", err)
		return
	}
	go session.DefaultSessionManager.Ping(currentSession)
}
