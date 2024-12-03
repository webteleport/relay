package relay

import (
	"expvar"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/btwiuse/connect"
	"github.com/btwiuse/forward"
	"github.com/webteleport/utils"
)

const ROOT_INTERNAL = "root.internal"

// h1/h2/h3 connect requests or forward requests
func IsProxy(r *http.Request) bool {
	if r.Header.Get("Proxy-Connection") != "" {
		return true
	}
	if r.Method == http.MethodConnect {
		return true
	}
	return false
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

var AuthenticatedProxyHandler = ConnectVerbose(ProxyAuthenticate(NewProxyHandler()))

func ProxyDispatcher(r *http.Request) http.Handler {
	switch {
	// HTTPS or WS/WSS target
	case r.Method == http.MethodConnect:
		return connect.Handler
	// Plain HTTP target
	default:
		return forward.Handler
	}
}

// TODO: handle ws://.internal CONNECT requests
func handleInternal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	msg := r.Method + " " + r.Host
	println(msg)
	http.Error(w, msg, http.StatusOK)
}

func IsInternal(r *http.Request) bool {
	return strings.HasSuffix(utils.StripPort(r.Host), ".internal")
}

func NewProxyHandler() http.Handler {
	return DispatcherFunc(ProxyDispatcher)
}

// ConnectVerbose is a misnomer
// It also logs plain HTTP proxy requests, which do not have CONNECT method
func ConnectVerbose(next http.Handler) http.Handler {
	if os.Getenv("CONNECT_VERBOSE") == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		println(r.Method, r.Proto, r.Host, r.Header)
		next.ServeHTTP(w, r)
	})
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

func (s *WSServer) RootInternalHandler(w http.ResponseWriter, r *http.Request) {
	if dbgvars := os.Getenv("INTERNAL_DEBUG_VARS_PATH"); dbgvars != "" && r.URL.Path == dbgvars {
		expvar.Handler().ServeHTTP(w, r)
		return
	}

	if apisess := os.Getenv("INTERNAL_API_SESSIONS_PATH"); apisess != "" && r.URL.Path == apisess {
		s.RecordsHandler(w, r)
		return
	}

	http.NotFound(w, r)
}

// rewrite requests targeting example.com/sub/* to sub.example.com/*
func (s *WSServer) RootHandler(w http.ResponseWriter, r *http.Request) {
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
