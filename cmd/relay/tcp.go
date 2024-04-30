package main

import (
	"net"

	"github.com/webteleport/relay"
)

func newTcpUpgrader(host string, port string) (*relay.TcpUpgrader, error) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, err
	}
	upgrader := &relay.TcpUpgrader{
		Listener: ln,
		HOST:     host,
	}
	return upgrader, nil
}
