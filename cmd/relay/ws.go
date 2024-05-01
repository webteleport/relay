package main

import (
	"context"
	"log"
	"net/http"

	"github.com/webteleport/relay"
	"github.com/webteleport/webteleport"
)

func newWebsocketUpgrader(host string, addr string) (*relay.WebsocketUpgrader, error) {
	ln, err := webteleport.Listen(context.Background(), addr)
	if err != nil {
		return nil, err
	}
	log.Println("Websocket server listening on https://" + ln.Addr().String())
	upgrader := &relay.WebsocketUpgrader{
		HOST: host,
	}
	go http.Serve(ln, upgrader)
	return upgrader, nil
}
