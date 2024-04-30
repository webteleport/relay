package main

import (
	"context"
	"log"
	"net/http"

	"github.com/webteleport/webteleport/transport/tcp"
)

func main() {
	ln, err := tcp.Listen(context.Background(), "127.0.0.1:8081/test?asdf=1")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	log.Println("Listening on", ln.Addr().Network(), ln.Addr().String())
	http.Serve(ln, nil)
}
