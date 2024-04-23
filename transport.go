package relay

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/webteleport/transport"
	"github.com/webteleport/utils"
)

func RoundTripper(tssn transport.Session) http.RoundTripper {
	dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
		expvars.WebteleportRelayStreamsSpawned.Add(1)
		stm, err := tssn.Open(ctx)
		return stm, err
	}
	return &http.Transport{
		DialContext:     dialCtx,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
}

func ReverseProxy(tssn transport.Session) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: RoundTripper(tssn),
		ErrorLog:  utils.ReverseProxyLogger(),
	}
}
