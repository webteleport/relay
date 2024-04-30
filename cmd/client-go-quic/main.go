// not working yet
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/webteleport/relay/cmd"
	"github.com/webteleport/webteleport/transport/go-quic"
)

func main() {
	log.SetFlags(log.Llongfile)
	ln, err := quic.Listen(context.Background(), cmd.Arg1("127.0.0.1:8083/test-go-quic?asdf=1"))
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	log.Println("Listening on", ln.Addr().Network(), ln.Addr().String())
	http.Serve(ln, nil)
}