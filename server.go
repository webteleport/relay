// root package development moved to github.com/webteleport/ufo/apps/server

package server

import (
	"log/slog"
	"net"
	"net/http"

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

func listenUDP(handler http.Handler, errc chan error) {
	slog.Info("listening on UDP https://" + envs.HOST + envs.PORT)
	wts := webteleport.NewServerTLS(handler, envs.CERT, envs.KEY)
	errc <- wts.ListenAndServe()
}

func listenAll(handler http.Handler) error {
	var errc chan error = make(chan error, 2)

	go listenTCP(handler, errc)
	go listenUDP(handler, errc)

	return <-errc
}

func Run([]string) error {
	var dsm http.Handler = session.DefaultSessionManager

	dsm = middleware.LoggingMiddleware(dsm)

	return listenAll(dsm)
}
