package main

import (
	"log/slog"
	"os"

	gq "github.com/webteleport/webteleport/transport/go-quic"
	"github.com/webtransport/quic"
)

// 2^60 == 1152921504606846976
var MaxBidiRemoteStreams int64 = 1 << 60

var GoQuicConfig = &quic.Config{
	TLSConfig:            TLSConfig,
	MaxBidiRemoteStreams: MaxBidiRemoteStreams,
	QLogLogger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     quic.QLogLevelFrame,
	})),
}

func newGoQuicUpgrader(host string, port string) (*gq.Upgrader, error) {
	qln, err := quic.Listen("udp", "0.0.0.0:"+port, GoQuicConfig)
	if err != nil {
		return nil, err
	}

	upgrader := &gq.Upgrader{
		Listener:     qln,
		RootPatterns: []string{host},
	}
	return upgrader, nil
}
