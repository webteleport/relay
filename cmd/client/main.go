package main

import (
	"fmt"
	"log"
	"os"

	"github.com/btwiuse/multicall"
)

func arg0(args []string, fallback string) string {
	if len(args) == 0 {
		return fallback
	}
	return args[0]
}

var cmdRun multicall.RunnerFuncMap = map[string]multicall.RunnerFunc{
	"tcp": RunTcp,
	"quic-go": RunQuicGo,
	"go-quic": RunGoQuic,
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	err := Run(os.Args)
	if err != nil {
		log.Fatalln(err)
	}
}

func Run(args []string) error {
	err := cmdRun.Run(os.Args[1:])
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
