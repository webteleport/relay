package relay

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/webteleport/webteleport/transport"
)

func Transport(tssn transport.Session) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			expvars.WebteleportRelayStreamsSpawned.Add(1)
			stm, err := tssn.OpenStream(ctx)
			return stm, err
		},
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
}
