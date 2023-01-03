package server

import (
	"log"
	"net"
	"net/http"

	"k0s.io/pkg/middleware"
)

func listenTCP(handler http.Handler, errc chan error) {
	log.Println("listening on TCP http://" + HOST + PORT)
	ln, err := net.Listen("tcp4", PORT)
	if err != nil {
		errc <- err
		return
	}
	errc <- http.Serve(ln, handler)
}

func listenUDP(handler http.Handler, errc chan error) {
	log.Println("listening on UDP https://" + HOST + PORT)
	wts := WebtransportServer(handler)
	errc <- wts.ListenAndServeTLS(CERT, KEY)
}

func listenAll(handler http.Handler) error {
	var errc chan error = make(chan error, 2)

	go listenTCP(handler, errc)
	go listenUDP(handler, errc)

	return <-errc
}

func Run([]string) error {
	var dsm http.Handler = DefaultSessionManager

	dsm = middleware.AllowAllCorsMiddleware(dsm)
	dsm = middleware.LoggingMiddleware(dsm)

	return listenAll(dsm)
}
