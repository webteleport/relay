package relay

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/btwiuse/wsconn"
	"github.com/hashicorp/yamux"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport"
	"github.com/webteleport/webteleport/transport/websocket"
)

func NewWSServer(host string, store Storage) *WSServer {
	return &WSServer{
		HOST:              host,
		Storage:           store,
		WebsocketUpgrader: &WebsocketUpgrader{},
		Proxy:             NewProxyHandler(),
	}
}

type WSServer struct {
	HOST string
	Storage
	*WebsocketUpgrader
	Proxy http.Handler
}

func (sm *WSServer) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("NAIVE") == "" || r.Header.Get("Naive") == "" {
		sm.Proxy.ServeHTTP(w, r)
		return
	}

	rhost, pw, okk := ProxyBasicAuth(r)
	tssn, ok := sm.Get(rhost)
	if !ok {
		slog.Warn(fmt.Sprintln("Proxy agent not found:", rhost, pw, okk))
		DefaultIndex().ServeHTTP(w, r)
		return
	}

	sm.Visited(rhost)

	if r.Header.Get("Host") == "" {
		r.Header.Set("Host", r.URL.Host)
	}

	proxyConnection := r.Header.Get("Proxy-Connection")
	proxyAuthorization := r.Header.Get("Proxy-Authorization")

	rw := func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
		req.Out.Header.Set("Proxy-Connection", proxyConnection)
		req.Out.Header.Set("Proxy-Authorization", proxyAuthorization)
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			expvars.WebteleportRelayStreamsSpawned.Add(1)
			stm, err := tssn.OpenStream(ctx)
			return stm, err
		},
		MaxIdleConns:       100,
		IdleConnTimeout:    90 * time.Second,
		DisableCompression: true,
	}
	rp := &httputil.ReverseProxy{
		Rewrite:   rw,
		Transport: tr,
	}
	println("proxy::open")
	// TODO: proxy request will stuck here
	// so for now this feature is not working
	rp.ServeHTTP(w, r)
	println("proxy::returned")
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

func DefaultIndex() http.Handler {
	handler := utils.HostNotFoundHandler()
	if index := utils.LookupEnv("INDEX"); index != nil {
		handler = utils.ReverseProxy(*index)
	}
	return utils.WellKnownHealthMiddleware(handler)
}

func leadingComponent(s string) string {
	return strings.Split(strings.TrimPrefix(s, "/"), "/")[0]
}

func (sm *WSServer) IndexHandler(w http.ResponseWriter, r *http.Request) {
	if dbgvars := os.Getenv("DEBUG_VARS_PATH"); dbgvars != "" && r.URL.Path == dbgvars {
		expvar.Handler().ServeHTTP(w, r)
		return
	}

	if apisess := os.Getenv("API_SESSIONS_PATH"); apisess != "" && r.URL.Path == apisess {
		sm.RecordsHandler(w, r)
		return
	}

	rpath := leadingComponent(r.URL.Path)
	rhost := fmt.Sprintf("%s.%s", rpath, sm.HOST)
	tssn, ok := sm.Get(rhost)
	if !ok {
		DefaultIndex().ServeHTTP(w, r)
		return
	}

	sm.Visited(rhost)

	rw := func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			expvars.WebteleportRelayStreamsSpawned.Add(1)
			stm, err := tssn.OpenStream(ctx)
			return stm, err
		},
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	rp := &httputil.ReverseProxy{
		Rewrite:   rw,
		Transport: tr,
	}
	http.StripPrefix("/"+rpath, rp).ServeHTTP(w, r)
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

func (sm *WSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isProxy := r.Header.Get("Proxy-Connection") != "" || r.Header.Get("Proxy-Authorization") != ""
	if isProxy && os.Getenv("CONNECT") != "" {
		sm.ConnectHandler(w, r)
		return
	}

	if sm.IsUpgrade(r) {
		tssn, tstm, err := sm.Upgrade(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade websocket session failed: %s", err))
			w.WriteHeader(500)
			return
		}

		key, err := sm.Negotiate(r, sm.HOST, tssn, tstm)
		if err != nil {
			slog.Warn(fmt.Sprintf("negotiate websocket session failed: %s", err))
			return
		}

		values := r.URL.Query()
		sm.Add(key, tssn, tstm, values)

		return
	}
	// for HTTP_PROXY r.Method = GET && r.Host = google.com
	// for HTTPs_PROXY r.Method = GET && r.Host = google.com:443
	// they are currently not supported and will be handled by the 404 handler
	if sm.IsIndex(r) {
		sm.IndexHandler(w, r)
		return
	}

	tssn, ok := sm.Get(r.Host)
	if !ok {
		utils.HostNotFoundHandler().ServeHTTP(w, r)
		return
	}

	sm.Visited(r.Host)

	rw := func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			expvars.WebteleportRelayStreamsSpawned.Add(1)
			stm, err := tssn.OpenStream(ctx)
			return stm, err
		},
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	rp := &httputil.ReverseProxy{
		Rewrite:   rw,
		Transport: tr,
	}
	rp.ServeHTTP(w, r)
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

func (sm *WSServer) IsIndex(r *http.Request) (result bool) {
	origin, _, _ := strings.Cut(r.Host, ":")
	return origin == sm.HOST
}

func (sm *WSServer) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-websocket-upgrade") != "" && sm.IsIndex(r)
}

type WebsocketUpgrader struct{}

func (*WebsocketUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (tssn transport.Session, tstm transport.Stream, err error) {
	conn, err := wsconn.Wrconn(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("upgrading failed: %s", err))
		w.WriteHeader(500)
		return
	}
	ssn, err := yamux.Server(conn, nil)
	if err != nil {
		slog.Warn(fmt.Sprintf("creating yamux.Server failed: %s", err))
		w.WriteHeader(500)
		return
	}
	tssn = &websocket.WebsocketSession{ssn}
	tstm, err = tssn.OpenStream(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("stm0 init failed: %s", err))
		return
	}
	return
}
