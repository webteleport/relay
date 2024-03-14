package manager

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/elazarl/goproxy/ext/auth"
)

func ProxyBasicAuth(r *http.Request) (username, password string, ok bool) {
	auth := r.Header.Get("Proxy-Authorization")
	if auth == "" {
		return
	}
	return parseProxyBasicAuth(auth)
}

func parseProxyBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
}

func NewProxyHandler() http.Handler {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = os.Getenv("CONNECT_VERBOSE") != ""

	// Create a BasicAuth middleware with the provided credentials
	basic := auth.BasicConnect(
		"Restricted",
		func(user, pass string) bool {
			ok := user != "" && pass != ""
			return ok
		},
	)

	// Use the BasicAuth middleware with the proxy server
	proxy.OnRequest().HandleConnect(basic)
	return proxy
}
