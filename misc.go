package relay

import (
	"expvar"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/btwiuse/connect"
	"github.com/webteleport/utils"
)

// when env CONNECT is set, filter authenticated h1/h2/h3 connect requests
func IsConnect(r *http.Request) bool {
	if os.Getenv("CONNECT") == "" {
		return false
	}
	if r.Method != http.MethodConnect {
		return false
	}
	return true
}

func ProxyAuthenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := ProxyBasicAuth(r)
		if !ok || !CheckProxyAuth(user, pass) {
			w.Header().Set("Proxy-Authenticate", fmt.Sprintf(`Basic realm="%s"`, r.Host))
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func CheckProxyAuth(user, pass string) bool {
	if user == "" || pass == "" {
		return false
	}
	return true
}

var AuthenticatedConnectHandler = ProxyAuthenticate(NewConnectHandler())

func NewConnectHandler() http.Handler {
	if os.Getenv("CONNECT_VERBOSE") != "" {
		return connectVerbose(connect.Handler)
	}
	return connect.Handler
}

func connectVerbose(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		println(r.Proto, r.Host, r.Header)
		next.ServeHTTP(w, r)
	})
}

func (s *WSServer) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("NAIVE") == "" || r.Header.Get("Naive") == "" {
		AuthenticatedConnectHandler.ServeHTTP(w, r)
		return
	}

	rhost, pw, okk := ProxyBasicAuth(r)
	rt, ok := s.GetRoundTripper(rhost)
	if !ok {
		slog.Warn(fmt.Sprintln("Proxy agent not found:", rhost, pw, okk))
		DefaultIndex().ServeHTTP(w, r)
		return
	}

	if r.Header.Get("Host") == "" {
		r.Header.Set("Host", r.URL.Host)
	}

	proxyConnection := r.Header.Get("Proxy-Connection")
	proxyAuthorization := r.Header.Get("Proxy-Authorization")

	rp := ReverseProxy(rt)
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

func (s *WTServer) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("NAIVE") == "" || r.Header.Get("Naive") == "" {
		AuthenticatedConnectHandler.ServeHTTP(w, r)
		return
	}
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
	rhost := fmt.Sprintf("%s.%s", rpath, s.HTTPUpgrader.Root())
	rt, ok := s.GetRoundTripper(rhost)
	if !ok {
		DefaultIndex().ServeHTTP(w, r)
		return
	}

	rp := ReverseProxy(rt)
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
