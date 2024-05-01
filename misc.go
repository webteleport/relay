package relay

import (
	"bytes"
	"expvar"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/btwiuse/connect"
	"github.com/btwiuse/tags"
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
	return r.Header.Get("Proxy-Authorization") != ""
}

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
		s.Connect.ServeHTTP(w, r)
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

func (s *WTServer) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("NAIVE") == "" || r.Header.Get("Naive") == "" {
		s.Connect.ServeHTTP(w, r)
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

func (s *WSServer) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	all := s.Records()
	filtered := []*Record{}
	for _, rec := range all {
		if rec.Matches(r.URL.Query()) {
			filtered = append(filtered, rec)
		}
	}
	resp, err := tags.UnescapedJSONMarshalIndent(filtered, "  ")
	if err != nil {
		slog.Warn(fmt.Sprintf("json marshal failed: %s", err))
		return
	}
	w.Write(resp)
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

func ReadLine(conn io.Reader) (string, error) {
	// do multiple read to get the first line
	b := make([]byte, 1)
	var buf bytes.Buffer
	for {
		_, err := conn.Read(b)
		if err != nil {
			return "", fmt.Errorf("read line error: %w", err)
		}
		if b[0] == '\n' {
			break
		}
		buf.Write(b)
	}
	return strings.TrimSpace(buf.String()), nil
}
