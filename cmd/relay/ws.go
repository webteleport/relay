package main

import (
	"context"
	"log"
	"net/http"

	"github.com/webteleport/webteleport"
	"github.com/webteleport/webteleport/transport/websocket"
)

func newWebsocketUpgrader(host string, addr string) (*websocket.Upgrader, error) {
	ln, err := webteleport.Listen(context.Background(), addr)
	if err != nil {
		return nil, err
	}
	log.Println("Websocket server listening on https://" + ln.Addr().String())
	upgrader := &websocket.Upgrader{
		HOST: host,
	}
	go http.Serve(ln, upgrader)
	return upgrader, nil
}
