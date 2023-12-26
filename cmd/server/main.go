//+build ignore

package main

import (
	"log"
	"os"

	"github.com/webteleport/server"

	_ "github.com/webteleport/utils/hack/quic-go-disable-receive-buffer-warning"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	err := server.Run(os.Args[1:])
	if err != nil {
		log.Fatalln(err)
	}
}
