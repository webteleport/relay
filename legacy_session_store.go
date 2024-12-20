// Deprecated: this file is largely replaced by the new store.go and ingress.go
// but it is kept here for reference, please do not modify this file
//
// TODO: remove this file after the spec is finalized
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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/btwiuse/muxr"
	"github.com/btwiuse/tags"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/edge"
	"github.com/webteleport/webteleport/tunnel"
	"golang.org/x/exp/maps"
	"golang.org/x/net/idna"
)

// Deprecated: NewSessionStore is aliased to NewIngress, use NewIngress instead
var NewSessionStore = NewIngress

var _ Storage = (*SessionStore)(nil)
var _ Ingress = (*SessionStore)(nil)

type SessionStore struct {
	*muxr.Router
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
	k = strings.Split(k, ".")[0]
	s.Lock.Lock()
	rec, ok := s.Record[k]
	if ok {
		rec.Visited += 1
	}
	s.Lock.Unlock()
}

func (s *SessionStore) RemoveSession(tssn tunnel.Session) {
	s.Lock.Lock()
	for _, rec := range s.Record {
		if rec.Session == tssn {
			delete(s.Record, rec.Key)
			if s.Verbose {
				slog.Info("remove", "key", rec.Key)
			}
			s.WebLog(fmt.Sprintf("remove/%s?ip=%s", rec.Key, rec.IP))
			break
		}
	}
	s.Lock.Unlock()
	expvars.WebteleportRelaySessionsClosed.Add(1)
}

func (s *SessionStore) GetSession(h string) (tunnel.Session, bool) {
	k := utils.StripPort(h)
	k, _ = idna.ToASCII(k)
	k = strings.Split(k, ".")[0]
	s.Lock.RLock()
	rec, ok := s.Record[k]
	s.Lock.RUnlock()
	if ok {
		return rec.Session, true
	}
	return nil, false
}

func (s *SessionStore) Upsert(k string, r *edge.Edge) {
	since := time.Now()
	header := tags.Tags{Values: url.Values(r.Header)}
	tags := tags.Tags{Values: r.Values}
	rec := &Record{
		Key:     k,
		Session: r.Session,
		Header:  header,
		Tags:    tags,
		Since:   since,
		Visited: 0,
		IP:      r.RealIP,
		Path:    r.Path,
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
		slog.Info(action, "key", rec.Key, "ip", rec.IP)
	}
	s.WebLog(fmt.Sprintf("%s/%s?ip=%s", action, rec.Key, rec.IP))

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
func (s *SessionStore) Ping(r *edge.Edge) {
	for {
		time.Sleep(s.PingInterval)
		_, err := io.WriteString(r.Stream, "\n")
		if err != nil {
			break
		}
	}
	s.RemoveSession(r.Session)
}

func (s *SessionStore) Scan(r *edge.Edge) {
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

func (s *SessionStore) Allocate(r *edge.Edge) (string, error) {
	k := deriveOnionID(r.Path)

	// Notify the client of the allocated hostname
	_, err := io.WriteString(r.Stream, fmt.Sprintf("HOST %s\n", k))
	if err != nil {
		return "", err
	}

	return k, nil
}

func (s *SessionStore) GetRoundTripper(h string) (http.RoundTripper, bool) {
	tssn, ok := s.GetSession(h)
	if !ok {
		return nil, false
	}
	return RoundTripper(tssn), true
}

func (s *SessionStore) Dispatch(r *http.Request) http.Handler {
	rt, ok := s.GetRoundTripper(r.Host)
	if !ok {
		return utils.HostNotFoundHandler()
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
	rp.ModifyResponse = func(resp *http.Response) error {
		s.Visited(r.Host)
		// TODO
		// is it ok to assume that the session is closed when the response is received?
		expvars.WebteleportRelayStreamsClosed.Add(1)
		return nil
	}
	return rp
}

func (s *SessionStore) Subscribe(upgrader edge.Upgrader) {
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

		key, err := s.Allocate(r)
		if err != nil {
			slog.Warn(fmt.Sprintf("allocate hostname failed: %s", err))
			continue
		}

		s.Upsert(key, r)
	}
}
