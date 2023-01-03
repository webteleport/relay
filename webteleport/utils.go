package webteleport

import (
	"net/http"
	"strings"

	"github.com/webteleport/server/envs"
)

// IsWebTeleportRequest tells if the incoming request should be treated as UFO request
//
// An UFO request must meet all criteria:
//
// - r.Proto == "webtransport"
// - r.Method == "CONNECT"
// - origin (r.Host without port) matches HOST
//
// if all true, it will be upgraded into a webtransport session
// otherwise the request will be handled by DefaultSessionManager
func IsWebTeleportRequest(r *http.Request) bool {
	var (
		origin, _, _ = strings.Cut(r.Host, ":")

		isWebtransport = r.Proto == "webtransport"
		isConnect      = r.Method == http.MethodConnect
		isOrigin       = origin == envs.HOST

		isWebTeleport = isWebtransport && isConnect && isOrigin
	)
	return isWebTeleport
}

// ParseDomainCandidates splits a path string like /a/b/cd/😏
// into a list of subdomains: [a, b, cd, 😏]
//
// when result is empty, a random subdomain will be assigned by the server
func ParseDomainCandidates(p string) []string {
	var list []string
	parts := strings.Split(p, "/")
	for _, part := range parts {
		dom := strings.Trim(part, " ")
		if dom == "" {
			continue
		}
		list = append(list, dom)
	}
	return list
}
