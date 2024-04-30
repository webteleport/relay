package main

import (
	"net"

	"github.com/quic-go/quic-go"
	"github.com/webteleport/relay"
)

// 2^60 == 1152921504606846976
var MaxIncomingStreams int64 = 1 << 60

var QuicGoConfig = &quic.Config{
	EnableDatagrams:    true,
	MaxIncomingStreams: MaxIncomingStreams,
}

func newQuicGoUpgrader(host string, port string) (*relay.QuicGoUpgrader, error) {
	addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:"+port)
	if err != nil {
		return nil, err
	}

	ln, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	qln, err := quic.Listen(ln, TLSConfig, QuicGoConfig)
	if err != nil {
		return nil, err
	}

	upgrader := &relay.QuicGoUpgrader{
		Listener: qln,
		HOST:     host,
	}
	return upgrader, nil
}
