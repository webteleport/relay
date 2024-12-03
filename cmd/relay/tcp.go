package main

import (
	"net"

	"github.com/webteleport/webteleport/transport/tcp"
)

func newTcpUpgrader(host string, port string) (*tcp.Upgrader, error) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, err
	}
	upgrader := &tcp.Upgrader{
		Listener:     ln,
		RootPatterns: []string{host},
	}
	return upgrader, nil
}
