package relay

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/tunnel"
)

func RoundTripper(tssn tunnel.Session) http.RoundTripper {
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

func ReverseProxy(rt http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: rt,
		ErrorLog:  utils.ReverseProxyLogger(),
	}
}
