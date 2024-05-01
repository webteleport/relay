package relay

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/btwiuse/rng"
	"github.com/btwiuse/tags"
	"github.com/webteleport/webteleport/spec"
	"github.com/webteleport/transport"
	"github.com/webteleport/utils"
	"golang.org/x/exp/maps"
	"golang.org/x/net/idna"
)

var _ Storage = (*SessionStore)(nil)

func NewSessionStore() *SessionStore {
	return &SessionStore{
		Lock:         &sync.RWMutex{},
		PingInterval: time.Second * 5,
		Verbose:      os.Getenv("VERBOSE") != "",
		Webhook:      os.Getenv("WEBHOOK"),
		Client:       &http.Client{},
		Record:       map[string]*Record{},
	}
}

type SessionStore struct {
	Lock         *sync.RWMutex
	PingInterval time.Duration
	Verbose      bool
	Webhook      string
	Client       *http.Client
	Record       map[string]*Record
}

func (s *SessionStore) WebLog(msg string) {
	if s.Webhook == "" {
		return
	}
	remote := fmt.Sprintf("%s/%s", s.Webhook, msg)
	req, err := http.NewRequest("LOG", remote, nil)
	if err != nil {
		return
	}
	go s.Client.Do(req)
}

type Record struct {
	Key     string            `json:"key"`
	Session transport.Session `json:"-"`
	Header  tags.Tags         `json:"header"`
	Tags    tags.Tags         `json:"tags"`
	Since   time.Time         `json:"since"`
	Visited int               `json:"visited"`
	IP      string            `json:"ip"`
}

func (r *Record) Matches(kvs url.Values) (ok bool) {
	for k, v := range kvs {
		// r.Tags contains k
		tv, has := r.Tags.Values[k]
		if !has {
			return false
		}
		// tv is superset of v
		for _, vv := range v {
			if !slices.Contains(tv, vv) {
				return false
			}
		}
	}
	return true
}

func (s *SessionStore) Records() (all []*Record) {
	s.Lock.RLock()
	all = maps.Values(s.Record)
	s.Lock.RUnlock()
	lessFunc := func(i, j int) bool {
		return all[i].Since.After(all[j].Since)
	}
	sort.Slice(all, lessFunc)
	return
}

func (s *SessionStore) RecordsHandler(w http.ResponseWriter, r *http.Request) {
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

func (s *SessionStore) Visited(k string) {
	k = utils.StripPort(k)
	k, _ = idna.ToASCII(k)
	s.Lock.Lock()
	rec, ok := s.Record[k]
	if ok {
		rec.Visited += 1
	}
	s.Lock.Unlock()
}

func (s *SessionStore) RemoveSession(tssn transport.Session) {
	s.Lock.Lock()
	for k, rec := range s.Record {
		if rec.Session == tssn {
			delete(s.Record, k)
			if s.Verbose {
				slog.Info("remove", "key", k)
			}
			s.WebLog(fmt.Sprintf("remove/%s", k))
			break
		}
	}
	s.Lock.Unlock()
	expvars.WebteleportRelaySessionsClosed.Add(1)
}

func (s *SessionStore) GetSession(k string) (transport.Session, bool) {
	k = utils.StripPort(k)
	k, _ = idna.ToASCII(k)
	s.Lock.RLock()
	rec, ok := s.Record[k]
	s.Lock.RUnlock()
	if ok {
		return rec.Session, true
	}
	return nil, false
}

func (s *SessionStore) Upsert(k string, r *spec.Request) {
	k = utils.StripPort(k)
	k, _ = idna.ToASCII(k)

	since := time.Now()
	header := tags.Tags{Values: url.Values(r.Header)}
	tags := tags.Tags{Values: r.Values}
	rec := &Record{
		Session: r.Session,
		Header:  header,
		Tags:    tags,
		Since:   since,
		Visited: 0,
		Key:     k,
		IP:      r.RealIP,
	}

	s.Lock.Lock()
	_, has := s.Record[k]
	s.Record[k] = rec
	s.Lock.Unlock()

	action := ""
	if has {
		action = "update"
	} else {
		action = "insert"
	}
	if s.Verbose {
		slog.Info(action, "key", k, "ip", rec.IP)
	}
	s.WebLog(fmt.Sprintf("%s/%s?ip=%s", action, k, rec.IP))

	if os.Getenv("PING") != "" {
		go s.Ping(r)
	}
	go s.Scan(r)

	expvars.WebteleportRelaySessionsAccepted.Add(1)
}

// Ping proactively pings the client to keep the connection alive and to detect if the client has disconnected.
// If the client has disconnected, the session is removed from the session store.
//
// This function has been found mostly unnecessary since the disconnect is automatically detected by the
// underlying transport layer and handled by the Scan function. However, it is kept here for completeness.
func (s *SessionStore) Ping(r *spec.Request) {
	for {
		time.Sleep(s.PingInterval)
		_, err := io.WriteString(r.Stream, "\n")
		if err != nil {
			break
		}
	}
	s.RemoveSession(r.Session)
}

func (s *SessionStore) Scan(r *spec.Request) {
	scanner := bufio.NewScanner(r.Stream)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "PONG" {
			// currently client reply nothing to server PING's
			// so this is a noop
			continue
		}
		if line == "CLOSE" {
			// close session immediately
			break
		}
		slog.Warn(fmt.Sprintf("stm0: unknown command: %s", line))
	}
	s.RemoveSession(r.Session)
}

func (s *SessionStore) Allocate(r *spec.Request, root string) (string, string, error) {
	var (
		candidates = utils.ParseDomainCandidates(r.Path)
		clobber    = r.Values.Get("clobber")
	)

	sub := ""
	if len(candidates) == 0 {
		sub = rng.NewDockerSepDigits("-", 4)
	} else {
		// Try to lease the first available subdomain if candidates are provided
		for _, pfx := range candidates {
			k := fmt.Sprintf("%s.%s", pfx, root)
			s.Lock.RLock()
			rec, exist := s.Record[k]
			s.Lock.RUnlock()

			insert := !exist
			updateByIP := exist && clobber == "" && rec.IP == r.RealIP
			updateByClobber := exist && clobber != "" && rec.Tags.Get("clobber") == clobber

			if upsert := insert || updateByIP || updateByClobber; upsert {
				sub = pfx
				break
			}
		}
	}
	if sub == "" {
		err := fmt.Errorf("none available: %v", candidates)
		return "", "", err
	}

	hostname := fmt.Sprintf("%s.%s", sub, root)
	hostnamePath := fmt.Sprintf("%s/%s/", root, sub)
	key := hostname

	if strings.HasSuffix(r.Path, "/") && r.Path != "/" {
		return key, hostnamePath, nil
	}
	return key, hostname, nil
}

func (s *SessionStore) Negotiate(r *spec.Request, root string) (string, error) {
	key, hp, err := s.Allocate(r, root)
	if err != nil {
		// Notify the client of the lease error
		_, err1 := io.WriteString(r.Stream, fmt.Sprintf("ERR %s\n", err))
		if err1 != nil {
			return "", err1
		}
		return "", err
	} else {
		// Notify the client of the hostname/path
		_, err1 := io.WriteString(r.Stream, fmt.Sprintf("HOST %s\n", hp))
		if err1 != nil {
			return "", err1
		}
	}
	return key, nil
}

func (s *SessionStore) GetRoundTripper(k string) (http.RoundTripper, bool) {
	tssn, ok := s.GetSession(k)
	if !ok {
		return nil, false
	}
	return RoundTripper(tssn), true
}

func (s *SessionStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt, ok := s.GetRoundTripper(r.Host)
	if !ok {
		utils.HostNotFoundHandler().ServeHTTP(w, r)
		return
	}

	s.Visited(r.Host)

	rp := ReverseProxy(rt)
	rp.Rewrite = func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
	}
	rp.ServeHTTP(w, r)
	expvars.WebteleportRelayStreamsClosed.Add(1)
}

func (s *SessionStore) Subscribe(upgrader spec.Upgrader) {
	for {
		r, err := upgrader.Upgrade()
		if err == io.EOF {
			slog.Warn("upgrade EOF")
			break
		}

		if err != nil {
			slog.Warn(fmt.Sprintf("upgrade session failed: %s", err))
			continue
		}

		if s.Verbose {
			slog.Info("subscribe", "request", r)
		}

		key, err := s.Negotiate(r, upgrader.Root())
		if err != nil {
			slog.Warn(fmt.Sprintf("negotiate session failed: %s", err))
			return
		}

		s.Upsert(key, r)
	}
}
