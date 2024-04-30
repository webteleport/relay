package main

import (
	"context"
	"log"
	"net/http"

	"github.com/webteleport/relay/cmd"
	"github.com/webteleport/webteleport/transport/quic-go"
)

func main() {
	ln, err := quic.Listen(context.Background(), cmd.Arg1("127.0.0.1:8082/test-quic-go?asdf=1"))
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	log.Println("Listening on", ln.Addr().Network(), ln.Addr().String())
	http.Serve(ln, nil)
}
