// not working yet
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/webteleport/webteleport/transport/go-quic"
)

func RunGoQuic(args []string) error {
	ln, err := quic.Listen(context.Background(), arg0(args, "127.0.0.1:8083/test-go-quic?asdf=1"))
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Println("Listening on", ln.Addr().Network(), ln.Addr().String())
	return http.Serve(ln, nil)
}
