package relay

import (
	"expvar"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/webteleport/utils"
)

const ROOT_INTERNAL = "root.internal"

// TODO: handle ws://.internal CONNECT requests
func handleInternal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	msg := r.Method + " " + r.Host
	http.Error(w, msg, http.StatusOK)
}

func IsInternal(r *http.Request) bool {
	return strings.HasSuffix(utils.StripPort(r.Host), ".internal")
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
	rt, ok := s.GetRoundTripper(rpath)
	if !ok {
		DefaultIndex().ServeHTTP(w, r)
		return
	}

	rp := utils.LoggedReverseProxy(rt)
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
