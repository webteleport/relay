package session

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/btwiuse/pretty"
	"github.com/webteleport/server/envs"
	"golang.org/x/net/idna"
)

var DefaultSessionManager = &SessionManager{
	counter:  0,
	sessions: map[string]*Session{},
	ssnstamp: map[string]time.Time{},
	slock:    &sync.RWMutex{},
}

type SessionManager struct {
	counter  int
	sessions map[string]*Session
	ssnstamp map[string]time.Time
	slock    *sync.RWMutex
}

func (sm *SessionManager) Del(k string) error {
	k, err := idna.ToASCII(k)
	if err != nil {
		return err
	}
	sm.slock.Lock()
	delete(sm.sessions, k)
	delete(sm.ssnstamp, k)
	sm.slock.Unlock()
	return nil
}

func (sm *SessionManager) DelSession(ssn *Session) {
	sm.slock.Lock()
	for k, v := range sm.sessions {
		if v == ssn {
			delete(sm.sessions, k)
			delete(sm.ssnstamp, k)
			emsg := fmt.Sprintf("Recycled %s", k)
			log.Println(emsg)
		}
	}
	sm.slock.Unlock()
}

func (sm *SessionManager) Get(k string) (*Session, bool) {
	k, _ = idna.ToASCII(k)
	host, _, _ := strings.Cut(k, ":")
	sm.slock.RLock()
	ssn, ok := sm.sessions[host]
	sm.slock.RUnlock()
	return ssn, ok
}

func (sm *SessionManager) Add(k string, ssn *Session) error {
	k, err := idna.ToASCII(k)
	if err != nil {
		return err
	}
	sm.slock.Lock()
	sm.counter += 1
	sm.sessions[k] = ssn
	sm.ssnstamp[k] = time.Now()
	sm.slock.Unlock()
	return nil
}

// canClobber checks if the clobber string matches the session's clobber value
func canClobber(ssn *Session, clobber string) bool {
	return clobber != "" && ssn.Values.Get("clobber") == clobber
}

func (sm *SessionManager) Lease(ssn *Session, candidates []string, clobber string) error {
	allowRandom := len(candidates) == 0
	leaseCandidate := ""

	// Try to lease the first available subdomain if candidates are provided
	for _, pfx := range candidates {
		k := fmt.Sprintf("%s.%s", pfx, envs.HOST)
		if ssn, exist := sm.Get(k); !exist || canClobber(ssn, clobber) {
			leaseCandidate = k
			break
		}
	}

	// If no specified candidates are available and random is not allowed, return with an error
	if leaseCandidate == "" && !allowRandom {
		emsg := fmt.Sprintf("ERR %s: %v\n", "none of your requested subdomains are currently available", candidates)
		_, err := io.WriteString(ssn.Controller, emsg)
		return err
	}

	// If no candidates were specified, generate a random subdomain
	if leaseCandidate == "" {
		leaseCandidate = fmt.Sprintf("%d.%s", sm.counter, envs.HOST)
	}

	// Notify the client of the leaseCandidate
	if _, err := io.WriteString(ssn.Controller, fmt.Sprintf("HOST %s\n", leaseCandidate)); err != nil {
		return err
	}

	// Add the leaseCandidate to the session manager
	if err := sm.Add(leaseCandidate, ssn); err != nil {
		return err
	}

	return nil
}

func (sm *SessionManager) Ping(ssn *Session) {
	for {
		_, err := io.WriteString(ssn.Controller, fmt.Sprintf("%s\n", "PING"))
		if err != nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
	sm.DelSession(ssn)
}

func (sm *SessionManager) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	NotFoundHandler().ServeHTTP(w, r)
}

type Record struct {
	Host      string     `json:"host"`
	CreatedAt time.Time  `json:"created_at"`
	Tags      url.Values `json:"tags"`
}

func (sm *SessionManager) ApiSessionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	all := []Record{}
	for host := range sm.sessions {
		since := sm.ssnstamp[host]
		tags := sm.sessions[host].Values
		record := Record{
			Host:      host,
			CreatedAt: since,
			Tags:      tags,
		}
		all = append(all, record)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	resp := pretty.JSONString(all)
	io.WriteString(w, resp)
}

func (sm *SessionManager) IndexHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/sessions":
		sm.ApiSessionsHandler(w, r)
	default:
		sm.NotFoundHandler(w, r)
	}
}

func (sm *SessionManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Alt-Svc", envs.ALT_SVC)
	// for HTTP_PROXY r.Method = GET && r.Host = google.com
	// for HTTPs_PROXY r.Method = GET && r.Host = google.com:443
	// they are currently not supported and will be handled by the 404 handler
	origin, _, _ := strings.Cut(r.Host, ":")
	if origin == envs.HOST {
		sm.IndexHandler(w, r)
		return
	}

	ssn, ok := sm.Get(r.Host)
	if !ok {
		sm.NotFoundHandler(w, r)
		return
	}

	dr := func(req *http.Request) {
		// log.Println("director: rewriting Host", r.URL, r.Host)
		req.Host = r.Host
		req.URL.Host = r.Host
		req.URL.Scheme = "http"
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Proto = r.Proto
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return ssn.OpenConn(ctx)
		},
	}
	rp := &httputil.ReverseProxy{
		Director:  dr,
		Transport: tr,
	}
	rp.ServeHTTP(w, r)
}
