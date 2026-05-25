package main

import (
	"net"
	"time"

	"github.com/quic-go/quic-go"
	qg "github.com/webteleport/webteleport/transport/quic-go"
)

// 2^60 == 1152921504606846976
var MaxIncomingStreams int64 = 1 << 60

var QuicGoConfig = &quic.Config{
	EnableDatagrams:                  true,
	EnableStreamResetPartialDelivery: true,
	MaxIncomingStreams:               MaxIncomingStreams,
	MaxIdleTimeout:                   30 * time.Second,
	KeepAlivePeriod:                  15 * time.Second,
}

func newQuicGoUpgrader(host string, port string) (*qg.Upgrader, error) {
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

	upgrader := &qg.Upgrader{
		Listener:     qln,
		RootPatterns: []string{host},
	}
	return upgrader, nil
}
