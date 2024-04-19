package relay

import (
	"expvar"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/webteleport/utils"
)

func (s *WSServer) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("NAIVE") == "" || r.Header.Get("Naive") == "" {
		s.Proxy.ServeHTTP(w, r)
		return
	}

	rhost, pw, okk := ProxyBasicAuth(r)
	tssn, ok := s.GetSession(rhost)
	if !ok {
		slog.Warn(fmt.Sprintln("Proxy agent not found:", rhost, pw, okk))
		DefaultIndex().ServeHTTP(w, r)
		return
	}

	s.Visited(rhost)

	if r.Header.Get("Host") == "" {
		r.Header.Set("Host", r.URL.Host)
	}

	proxyConnection := r.Header.Get("Proxy-Connection")
	proxyAuthorization := r.Header.Get("Proxy-Authorization")

	rp := ReverseProxy(tssn)
	rp.Rewrite = func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
		req.Out.Header.Set("Proxy-Connection", proxyConnection)
		req.Out.Header.Set("Proxy-Authorization", proxyAuthorization)
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

func (s *WSServer) IndexHandler(w http.ResponseWriter, r *http.Request) {
	if dbgvars := os.Getenv("DEBUG_VARS_PATH"); dbgvars != "" && r.URL.Path == dbgvars {
		expvar.Handler().ServeHTTP(w, r)
		return
	}

	if apisess := os.Getenv("API_SESSIONS_PATH"); apisess != "" && r.URL.Path == apisess {
		s.RecordsHandler(w, r)
		return
	}

	rpath := leadingComponent(r.URL.Path)
	rhost := fmt.Sprintf("%s.%s", rpath, s.HOST)
	tssn, ok := s.GetSession(rhost)
	if !ok {
		DefaultIndex().ServeHTTP(w, r)
		return
	}

	s.Visited(rhost)

	rp := ReverseProxy(tssn)
	rp.Rewrite = func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
	}
	http.StripPrefix("/"+rpath, rp).ServeHTTP(w, r)
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

func StripPort(hostport string) string {
	// use net.SplitHostPort instead of strings.Split
	// because it can handle ipv6 addresses
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		// if there is no port, just return the input
		return hostport
	}
	return host
}
