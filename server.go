// root package development moved to github.com/webteleport/ufo/apps/server

package server

import (
	"log/slog"
	"net"
	"net/http"

	"github.com/caddyserver/certmagic"
	"github.com/webteleport/server/envs"
	"github.com/webteleport/server/session"
	"github.com/webteleport/server/webteleport"
	"k0s.io/pkg/middleware"
)

func listenTCP(handler http.Handler, errc chan error) {
	slog.Info("listening on TCP http://" + envs.HOST + envs.PORT)
	ln, err := net.Listen("tcp4", envs.PORT)
	if err != nil {
		errc <- err
		return
	}
	errc <- http.Serve(ln, handler)
}

func listenTCPOnDemandTLS(handler http.Handler, errc chan error) {
	slog.Info("listening on TCP https://" + envs.HOST + envs.PORT + " w/ on demand tls")
	// Because this convenience function returns only a TLS-enabled
	// listener and does not presume HTTP is also being served,
	// the HTTP challenge will be disabled. The package variable
	// Default is modified so that the HTTP challenge is disabled.
	certmagic.Default.DisableHTTPChallenge = true
	ln, err := tls.Listen("tcp4", ":443", certmagic.Default.TLSConfig())
	ln, err := net.Listen("tcp4", envs.PORT)
	if err != nil {
		errc <- err
		return
	}
	errc <- http.Serve(ln, handler)
}

func listenUDP(handler http.Handler, errc chan error) {
	slog.Info("listening on UDP https://" + envs.HOST + envs.PORT)
	wts := webteleport.NewServerTLS(handler, envs.CERT, envs.KEY)
	errc <- wts.ListenAndServe()
}

func listenUDPOnDemandTLS(handler http.Handler, errc chan error) {
	slog.Info("listening on UDP https://" + envs.HOST + envs.PORT + " w/ on demand tls")
	wts := webteleport.NewServerTLSOnDemand(handler)
	errc <- wts.ListenAndServe()
}

func listenAll(handler http.Handler) error {
	var errc chan error = make(chan error, 2)

	go listenTCPOnDemandTLS(handler, errc)
	go listenUDPOnDemandTLS(handler, errc)

	return <-errc
}

func Run([]string) error {
	var dsm http.Handler = session.DefaultSessionManager

	dsm = middleware.LoggingMiddleware(dsm)

	return listenAll(dsm)
}
