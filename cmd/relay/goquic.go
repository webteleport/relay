package main

import (
	"github.com/webteleport/relay"
	"github.com/webtransport/quic"
)

// 2^60 == 1152921504606846976
var MaxBidiRemoteStreams int64 = 1 << 60

var GoQuicConfig = &quic.Config{
	TLSConfig:            TLSConfig,
	MaxBidiRemoteStreams: MaxBidiRemoteStreams,
}

func newGoQuicUpgrader(host string, port string) (*relay.GoQuicUpgrader, error) {
	qln, err := quic.Listen("udp", ":"+port, GoQuicConfig)
	if err != nil {
		return nil, err
	}

	upgrader := &relay.GoQuicUpgrader{
		Listener: qln,
		HOST:     host,
	}
	return upgrader, nil
}
