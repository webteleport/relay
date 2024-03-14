package manager

import (
	"bufio"
	"context"
	"expvar"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/btwiuse/rng"
	"github.com/btwiuse/tags"
	"github.com/hashicorp/yamux"
	"github.com/webteleport/relay/session"
	"github.com/webteleport/utils"
	"golang.org/x/net/idna"
	"k0s.io/pkg/wrap"
)

var _ Session = (*session.WebsocketSession)(nil)
var _ Session = (*session.WebtransportSession)(nil)

type Session interface {
	InitController(context.Context) error
	GetController() net.Conn
	GetValues() url.Values
	OpenConn(context.Context) (net.Conn, error)
}

var DefaultSessionManager = &SessionManager{
	HOST:     "<unknown>",
	counter:  0,
	sessions: map[string]Session{},
	ssnstamp: map[string]time.Time{},
	ssn_cntr: map[string]int{},
	slock:    &sync.RWMutex{},
	proxy:    NewProxyHandler(),
}

type SessionManager struct {
	HOST     string
	counter  int
	sessions map[string]Session
	ssnstamp map[string]time.Time
	ssn_cntr map[string]int
	slock    *sync.RWMutex
	proxy    http.Handler
}

func (sm *SessionManager) DelSession(ssn Session) {
	sm.slock.Lock()
	for k, v := range sm.sessions {
		if v == ssn {
			delete(sm.sessions, k)
			delete(sm.ssnstamp, k)
			delete(sm.ssn_cntr, k)
			emsg := fmt.Sprintf("Recycled %s", k)
			slog.Info(emsg)
		}
	}
	sm.slock.Unlock()
	expvars.WebteleportRelaySessionsClosed.Add(1)
}

func (sm *SessionManager) Get(k string) (Session, bool) {
	k, _ = idna.ToASCII(k)
	host, _, _ := strings.Cut(k, ":")
	sm.slock.RLock()
	ssn, ok := sm.sessions[host]
	sm.slock.RUnlock()
	return ssn, ok
}

func (sm *SessionManager) Add(k string, ssn Session) error {
	k, err := idna.ToASCII(k)
	if err != nil {
		return err
	}
	sm.slock.Lock()
	sm.counter += 1
	sm.sessions[k] = ssn
	sm.ssnstamp[k] = time.Now()
	sm.ssn_cntr[k] = 0
	sm.slock.Unlock()
	return nil
}

// canClobber checks if the clobber string matches the session's clobber value
func canClobber(ssn Session, clobber string) bool {
	return clobber != "" && ssn.GetValues().Get("clobber") == clobber
}

func (sm *SessionManager) Lease(r *http.Request, ssn Session, candidates []string, clobber string) error {
	allowRandom := len(candidates) == 0
	leaseCandidate := ""

	// Try to lease the first available subdomain if candidates are provided
	for _, pfx := range candidates {
		k := fmt.Sprintf("%s.%s", pfx, sm.HOST)
		if ssn, exist := sm.Get(k); !exist || canClobber(ssn, clobber) {
			leaseCandidate = pfx
			break
		}
	}

	// If no specified candidates are available and random is not allowed, return with an error
	if leaseCandidate == "" && !allowRandom {
		emsg := fmt.Sprintf("ERR %s: %v\n", "none of your requested subdomains are currently available", candidates)
		_, err := io.WriteString(ssn.GetController(), emsg)
		return err
	}

	// If no candidates were specified, generate a random subdomain
	if leaseCandidate == "" {
		leaseCandidate = rng.NewDockerSepDigits("-", 4)
	}

	hostname := fmt.Sprintf("%s.%s", leaseCandidate, sm.HOST)
	hostnamePath := fmt.Sprintf("%s/%s/", sm.HOST, leaseCandidate)

	reply := fmt.Sprintf("HOST %s\n", hostname)
	if strings.HasSuffix(r.URL.Path, "/") && r.URL.Path != "/" {
		reply = fmt.Sprintf("HOST %s\n", hostnamePath)
	}

	// Notify the client of the leaseCandidate
	if _, err := io.WriteString(ssn.GetController(), reply); err != nil {
		return err
	}

	// Add the leaseCandidate to the session manager
	if err := sm.Add(hostname, ssn); err != nil {
		return err
	}

	return nil
}

var PingInterval = 5 * time.Second

func (sm *SessionManager) Ping(ssn Session) {
	for {
		time.Sleep(PingInterval)
		_, err := io.WriteString(ssn.GetController(), fmt.Sprintf("%s\n", "PING"))
		if err != nil {
			break
		}
	}
	sm.DelSession(ssn)
}

func (sm *SessionManager) Scan(ssn Session) {
	scanner := bufio.NewScanner(ssn.GetController())
	for scanner.Scan() {
		line := scanner.Text()
		if line == "PONG" {
			// currently client reply nothing to server PING's
			// so this is a noop
			continue
		}
		if line == "CLOSE" {
			// close session immediately
			sm.DelSession(ssn)
			break
		}
		slog.Warn(fmt.Sprintf("stm0: unknown command: %s", line))
	}
}

func (sm *SessionManager) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("NAIVE") == "" || r.Header.Get("Naive") == "" {
		sm.proxy.ServeHTTP(w, r)
		return
	}

	rhost, pw, okk := ProxyBasicAuth(r)
	ssn, ok := sm.Get(rhost)
	if !ok {
		slog.Warn(fmt.Sprintln("Proxy agent not found:", rhost, pw, okk))
		Index().ServeHTTP(w, r)
		return
	}

	sm.IncrementVisit(rhost)

	if r.Header.Get("Host") == "" {
		r.Header.Set("Host", r.URL.Host)
	}

	proxyConnection := r.Header.Get("Proxy-Connection")
	proxyAuthorization := r.Header.Get("Proxy-Authorization")

	rw := func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
		req.Out.Header.Set("Proxy-Connection", proxyConnection)
		req.Out.Header.Set("Proxy-Authorization", proxyAuthorization)
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			expvars.WebteleportRelayStreamsSpawned.Add(1)
			return ssn.OpenConn(ctx)
		},
		MaxIdleConns:       100,
		IdleConnTimeout:    90 * time.Second,
		DisableCompression: true,
	}
	rp := &httputil.ReverseProxy{
		Rewrite:   rw,
		Transport: tr,
	}
	println("proxy::open")
	// TODO: proxy request will stuck here
	// so for now this feature is not working
	rp.ServeHTTP(w, r)
	println("proxy::returned")
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

type Record struct {
	Host      string    `json:"host"`
	CreatedAt time.Time `json:"created_at"`
	Tags      tags.Tags `json:"tags"`
	Visited   int       `json:"visited"`
}

func (sm *SessionManager) ApiSessionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	all := []Record{}
	for host := range sm.sessions {
		since := sm.ssnstamp[host]
		tags := tags.Tags{Values: sm.sessions[host].GetValues()}
		visited := sm.ssn_cntr[host]
		record := Record{
			Host:      host,
			CreatedAt: since,
			Tags:      tags,
			Visited:   visited,
		}
		all = append(all, record)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	resp, err := tags.UnescapedJSONMarshalIndent(all, "  ")
	if err != nil {
		slog.Warn(fmt.Sprintf("json marshal failed: %s", err))
		return
	}
	w.Write(resp)
}

func Index() http.Handler {
	handler := utils.HostNotFoundHandler()
	if index := utils.LookupEnv("INDEX"); index != nil {
		handler = utils.ReverseProxy(*index)
	}
	return utils.WellKnownHealthMiddleware(handler)
}

func leadingComponent(s string) string {
	return strings.Split(strings.TrimPrefix(s, "/"), "/")[0]
}

func (sm *SessionManager) IndexHandler(w http.ResponseWriter, r *http.Request) {
	if dbgvars := os.Getenv("DEBUG_VARS_PATH"); dbgvars != "" && r.URL.Path == dbgvars {
		expvar.Handler().ServeHTTP(w, r)
		return
	}

	if apisess := os.Getenv("API_SESSIONS_PATH"); dbgvars != "" && r.URL.Path == apisess {
		sm.ApiSessionsHandler(w, r)
		return
	}

	rpath := leadingComponent(r.URL.Path)
	rhost := fmt.Sprintf("%s.%s", rpath, sm.HOST)
	ssn, ok := sm.Get(rhost)
	if !ok {
		Index().ServeHTTP(w, r)
		return
	}

	sm.IncrementVisit(rhost)

	rw := func(req *httputil.ProxyRequest) {
		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			expvars.WebteleportRelayStreamsSpawned.Add(1)
			return ssn.OpenConn(ctx)
		},
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	rp := &httputil.ReverseProxy{
		Rewrite:   rw,
		Transport: tr,
	}
	http.StripPrefix("/"+rpath, rp).ServeHTTP(w, r)
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

func (sm *SessionManager) IncrementVisit(k string) {
	sm.slock.Lock()
	sm.ssn_cntr[k] += 1
	sm.slock.Unlock()
}

func (sm *SessionManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isProxy := r.Header.Get("Proxy-Connection") != "" || r.Header.Get("Proxy-Authorization") != ""
	if isProxy && os.Getenv("CONNECT") != "" {
		sm.ConnectHandler(w, r)
		return
	}

	if IsWebsocketUpgrade(r) {
		currentSession, err := UpgradeWebsocketSession(w, r)
		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade websocket session failed: %s", err))
			return
		}
		AddManagerSession(currentSession, r)
		return
	}
	// for HTTP_PROXY r.Method = GET && r.Host = google.com
	// for HTTPs_PROXY r.Method = GET && r.Host = google.com:443
	// they are currently not supported and will be handled by the 404 handler
	origin, _, _ := strings.Cut(r.Host, ":")
	if origin == sm.HOST {
		sm.IndexHandler(w, r)
		return
	}

	ssn, ok := sm.Get(r.Host)
	if !ok {
		utils.HostNotFoundHandler().ServeHTTP(w, r)
		return
	}

	sm.IncrementVisit(r.Host)

	rw := func(req *httputil.ProxyRequest) {
		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			expvars.WebteleportRelayStreamsSpawned.Add(1)
			return ssn.OpenConn(ctx)
		},
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	rp := &httputil.ReverseProxy{
		Rewrite:   rw,
		Transport: tr,
	}
	rp.ServeHTTP(w, r)
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

func AddManagerSession(currentSession Session, r *http.Request) {
	var (
		candidates = utils.ParseDomainCandidates(r.URL.Path)
		clobber    = r.URL.Query().Get("clobber")
	)

	if err := DefaultSessionManager.Lease(r, currentSession, candidates, clobber); err != nil {
		slog.Warn(fmt.Sprintf("leasing failed: %s", err))
		return
	}
	go DefaultSessionManager.Ping(currentSession)
	go DefaultSessionManager.Scan(currentSession)
	expvars.WebteleportRelaySessionsAccepted.Add(1)
}

func UpgradeWebsocketSession(w http.ResponseWriter, r *http.Request) (Session, error) {
	conn, err := wrap.Wrconn(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("upgrading failed: %s", err))
		w.WriteHeader(500)
		return nil, err
	}
	ssn, err := yamux.Server(conn, nil)
	if err != nil {
		slog.Warn(fmt.Sprintf("creating yamux.Server failed: %s", err))
		w.WriteHeader(500)
		return nil, err
	}
	managerSession := &session.WebsocketSession{
		Session: ssn,
		Values:  r.URL.Query(),
	}
	err = managerSession.InitController(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("session init failed: %s", err))
		return nil, err
	}
	return managerSession, nil
}

func IsWebsocketUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-websocket-upgrade") != ""
}
