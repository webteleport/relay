package main

import (
	"log/slog"
	"os"

	nq "github.com/webteleport/webteleport/transport/net-quic"
	"golang.org/x/net/quic"
)

// 2^60 == 1152921504606846976
var MaxBidiRemoteStreams int64 = 1 << 60

var NetQuicConfig = &quic.Config{
	TLSConfig:            TLSConfig,
	MaxBidiRemoteStreams: MaxBidiRemoteStreams,
	QLogLogger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     quic.QLogLevelFrame,
	})),
}

func newNetQuicUpgrader(host string, port string) (*nq.Upgrader, error) {
	qln, err := quic.Listen("udp", "0.0.0.0:"+port, NetQuicConfig)
	if err != nil {
		return nil, err
	}

	upgrader := &nq.Upgrader{
		Listener:     qln,
		RootPatterns: []string{host},
	}
	return upgrader, nil
}
