package relay

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/btwiuse/tags"
	"github.com/phayes/freeport"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/edge"
	"github.com/webteleport/webteleport/tunnel"
	"golang.org/x/exp/maps"
	"golang.org/x/net/idna"
)

var _ Storage = (*Store)(nil)

var DefaultStorage = NewStore()

type Store struct {
	OnUpdateFunc func(* Store)
	Lock         *sync.RWMutex
	PingInterval time.Duration
	Verbose      bool
	Webhook      string
	Client       *http.Client
	RecordMap    map[string]*Record
	AliasMap     map[string]string
}

func NewStore() *Store {
	return &Store{
		Lock:         &sync.RWMutex{},
		PingInterval: time.Second * 5,
		Verbose:      os.Getenv("VERBOSE") != "",
		Webhook:      os.Getenv("WEBHOOK"),
		Client:       &http.Client{},
		RecordMap:    map[string]*Record{},
		AliasMap:     map[string]string{},
	}
}

func (s *Store) OnUpdate() {
	if s.OnUpdateFunc != nil {
		return
	}
	s.OnUpdateFunc(s)
}

func (s *Store) Records() (all []*Record) {
	s.Lock.RLock()
	all = maps.Values(s.RecordMap)
	s.Lock.RUnlock()
	sort.Slice(all, func(i, j int) bool {
		return all[i].Since.After(all[j].Since)
	})
	return
}

func (s *Store) Alias(k string, v string) {
	defer s.OnUpdate()
	s.Lock.Lock()
	s.AliasMap[k] = v
	s.Lock.Unlock()
}

func (s *Store) Unalias(k string) {
	defer s.OnUpdate()
	s.Lock.Lock()
	delete(s.AliasMap, k)
	s.Lock.Unlock()
}

func (s *Store) Aliases() (all map[string]string) {
	s.Lock.RLock()
	all = s.AliasMap
	s.Lock.RUnlock()
	return
}

// lookup record by key, or alias
func (s *Store) LookupRecord(k string) (rec *Record, ok bool) {
	s.Lock.RLock()
	rec, ok = s.RecordMap[k]
	if !ok {
		k, ok = s.AliasMap[k]
		if ok {
			rec, ok = s.RecordMap[k]
		}
	}
	s.Lock.RUnlock()
	return
}

func (s *Store) WebLog(msg string) {
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

func (s *Store) RemoveSession(tssn tunnel.Session) {
	defer s.OnUpdate()
	s.Lock.Lock()
	for _, rec := range s.RecordMap {
		if rec.Session == tssn {
			delete(s.RecordMap, rec.Key)
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

func (s *Store) GetRecord(h string) (*Record, bool) {
	k := utils.StripPort(h)
	k, _ = idna.ToASCII(k)
	k = strings.Split(k, ".")[0]
	rec, ok := s.LookupRecord(k)
	if ok {
		return rec, true
	}
	return nil, false
}

func (s *Store) Allocate(r *edge.Edge) (key string, err error) {
	switch edgeProtocol(r) {
	case "tcp":
		key, err = s.allocateTCP(r)
	default:
		key, err = s.allocateHTTP(r)
	}
	return
}

func (s *Store) allocateTCP(r *edge.Edge) (string, error) {
	port, err := freeport.GetFreePort()
	if err != nil {
		return "", fmt.Errorf("failed to allocate tcp port: %w", err)
	}
	return fmt.Sprintf(":%d", port), nil
}

func (s *Store) allocateHTTP(r *edge.Edge) (string, error) {
	k := deriveOnionID(r.Path)
	return k, nil
}

func (s *Store) Upsert(k string, r *edge.Edge) {
	since := time.Now()
	header := tags.Tags{Values: url.Values(r.Header)}
	tags := tags.Tags{Values: r.Values}
	rec := &Record{
		Key:     k,
		Session: r.Session,
		Header:  header,
		Tags:    tags,
		Since:   since,
		IP:      r.RealIP,
		Path:    r.Path,
	}

	if edgeProtocol(r) == "http" {
		rec.RoundTripper = RoundTripper(r.Session)
	}

	defer s.OnUpdate()
	s.Lock.Lock()
	_, has := s.RecordMap[k]
	s.RecordMap[k] = rec
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

func (s *Store) Ping(r *edge.Edge) {
	for {
		time.Sleep(s.PingInterval)
		_, err := io.WriteString(r.Stream, "\n")
		if err != nil {
			break
		}
	}
	s.RemoveSession(r.Session)
}

func (s *Store) Scan(r *edge.Edge) {
	scanner := bufio.NewScanner(r.Stream)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "PONG" {
			continue
		}
		if line == "CLOSE" {
			break
		}
		slog.Warn(fmt.Sprintf("stm0: unknown command: %s", line))
	}
	s.RemoveSession(r.Session)
}

func (s *Store) Subscribe(upgrader edge.Upgrader) {
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
			slog.Warn(fmt.Sprintf("allocate resource failed: %s", err))
			_, _ = io.WriteString(r.Stream, fmt.Sprintf("ERR %s\n", err))
			continue
		}

		_, _ = io.WriteString(r.Stream, fmt.Sprintf("HOST %s\n", key))

		s.Upsert(key, r)
	}
}

func edgeProtocol(r *edge.Edge) string {
	protocol := r.Values.Get("protocol")
	if protocol != "" {
		return protocol
	}
	return "http"
}
