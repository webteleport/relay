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
	switch r.Method {
	case http.MethodConnect:
		return connect.Handler
	default:
		return forward.Handler
	}
}

func NewProxyHandler() http.Handler {
	return DispatcherFunc(ProxyDispatcher)
}

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
