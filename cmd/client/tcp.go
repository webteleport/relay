package main

import (
	"context"
	"log"
	"net/http"

	"github.com/webteleport/webteleport/transport/tcp"
)

func RunTcp(args []string) error {
	ln, err := tcp.Listen(context.Background(), arg0(args, "127.0.0.1:8081/test?asdf=1"))
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Println("Listening on", ln.Addr().Network(), ln.Addr().String())
	return http.Serve(ln, nil)
}
