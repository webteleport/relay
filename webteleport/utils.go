package webteleport

import (
	"strings"
)

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
