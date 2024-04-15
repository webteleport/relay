package relay

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/btwiuse/rng"
	"github.com/btwiuse/tags"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport"
	"golang.org/x/exp/maps"
	"golang.org/x/net/idna"
)

func NewSessionStore() *SessionStore {
	return &SessionStore{
		Lock:         &sync.RWMutex{},
		PingInterval: time.Second * 5,
		Record:       map[string]*Record{},
	}
}

type SessionStore struct {
	Lock         *sync.RWMutex
	PingInterval time.Duration
	Record       map[string]*Record
}

type Record struct {
	Key     string            `json:"key"`
	Session transport.Session `json:"-"`
	Tags    tags.Tags         `json:"tags"`
	Since   time.Time         `json:"since"`
	Visited int               `json:"visited"`
}

func (s *SessionStore) Records() (all []*Record) {
	all = maps.Values(s.Record)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Since.After(all[j].Since)
	})
	return
}

func (s *SessionStore) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	all := s.Records()
	resp, err := tags.UnescapedJSONMarshalIndent(all, "  ")
	if err != nil {
		slog.Warn(fmt.Sprintf("json marshal failed: %s", err))
		return
	}
	w.Write(resp)
}

func (s *SessionStore) Visited(k string) {
	s.Lock.Lock()
	rec, ok := s.Record[k]
	if ok {
		rec.Visited += 1
	}
	s.Lock.Unlock()
}

func (s *SessionStore) Remove(k string) {
	s.Lock.Lock()
	delete(s.Record, k)
	s.Lock.Unlock()
	emsg := fmt.Sprintf("Recycled %s", k)
	slog.Info(emsg)
	expvars.WebteleportRelaySessionsClosed.Add(1)
}

func (s *SessionStore) Get(k string) (transport.Session, bool) {
	k, _ = idna.ToASCII(k)
	host, _, _ := strings.Cut(k, ":")
	s.Lock.RLock()
	rec, ok := s.Record[host]
	s.Lock.RUnlock()
	if ok {
		return rec.Session, true
	}
	return nil, false
}

func (s *SessionStore) Add(k string, tssn transport.Session, tstm transport.Stream, r *http.Request) {
	k, _ = idna.ToASCII(k)
	s.Lock.Lock()

	since := time.Now()
	tags := tags.Tags{Values: r.URL.Query()}
	rec := &Record{
		Session: tssn,
		Tags:    tags,
		Since:   since,
		Visited: 0,
		Key:     k,
	}
	s.Record[k] = rec

	s.Lock.Unlock()

	go s.Ping(k, tstm)
	go s.Scan(k, tstm)

	expvars.WebteleportRelaySessionsAccepted.Add(1)
}

func (s *SessionStore) Ping(k string, tstm transport.Stream) {
	for {
		time.Sleep(s.PingInterval)
		_, err := io.WriteString(tstm, fmt.Sprintf("%s\n", "PING"))
		if err != nil {
			break
		}
	}
	s.Remove(k)
}

func (s *SessionStore) Scan(k string, tstm transport.Stream) {
	scanner := bufio.NewScanner(tstm)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "PONG" {
			// currently client reply nothing to server PING's
			// so this is a noop
			continue
		}
		if line == "CLOSE" {
			// close session immediately
			s.Remove(k)
			break
		}
		slog.Warn(fmt.Sprintf("stm0: unknown command: %s", line))
	}
}

func (s *SessionStore) Allocate(r *http.Request, root string) (string, string, error) {
	var (
		candidates = utils.ParseDomainCandidates(r.URL.Path)
		Values     = r.URL.Query()
		clobber    = Values.Get("clobber")
	)

	sub := ""
	if len(candidates) == 0 {
		sub = rng.NewDockerSepDigits("-", 4)
	} else {
		// Try to lease the first available subdomain if candidates are provided
		for _, pfx := range candidates {
			k := fmt.Sprintf("%s.%s", pfx, root)
			rec, exist := s.Record[k]
			if !exist || (clobber != "" && rec.Tags.Get("clobber") == clobber) {
				sub = pfx
				break
			}
		}
	}
	if sub == "" {
		err := fmt.Errorf("none of your requested subdomains are currently available: %v", candidates)
		return "", "", err
	}

	hostname := fmt.Sprintf("%s.%s", sub, root)
	hostnamePath := fmt.Sprintf("%s/%s/", root, sub)
	key := hostname

	if strings.HasSuffix(r.URL.Path, "/") && r.URL.Path != "/" {
		return key, hostnamePath, nil
	}
	return key, hostname, nil
}

func (s *SessionStore) Negotiate(r *http.Request, root string, tssn transport.Session, tstm transport.Stream) (string, error) {
	key, hp, err := s.Allocate(r, root)
	if err != nil {
		// Notify the client of the lease error
		_, err1 := io.WriteString(tstm, fmt.Sprintf("ERR %s\n", err))
		if err1 != nil {
			return "", err1
		}
		return "", err
	} else {
		// Notify the client of the hostname/path
		_, err1 := io.WriteString(tstm, fmt.Sprintf("HOST %s\n", hp))
		if err1 != nil {
			return "", err1
		}
	}
	return key, nil
}

func (s *SessionStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tssn, ok := s.Get(r.Host)
	if !ok {
		utils.HostNotFoundHandler().ServeHTTP(w, r)
		return
	}

	s.Visited(r.Host)

	rw := func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		// for webtransport, Proto is "webtransport" instead of "HTTP/1.1"
		// However, reverseproxy doesn't support webtransport yet
		// so setting this field currently doesn't have any effect
		req.Out.URL.Scheme = "http"
	}
	rp := &httputil.ReverseProxy{
		Rewrite:   rw,
		Transport: Transport(tssn),
	}
	rp.ServeHTTP(w, r)
	expvars.WebteleportRelayStreamsClosed.Add(1)
}
