package relay

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/webteleport/webteleport/tunnel"
)

func RoundTripper(tssn tunnel.Session) http.RoundTripper {
	dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
		expvars.WebteleportRelayStreamsSpawned.Add(1)
		stm, err := tssn.Open(ctx)
		return &VerboseConn{Conn: stm}, err
	}
	tr := &http.Transport{
		DialContext:     dialCtx,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	return NewMetricsTransport(tr)
}
