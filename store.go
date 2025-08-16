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
	OnUpdateFunc func(*Store)
	Logger       *slog.Logger
	Lock         *sync.RWMutex
	PingInterval time.Duration
	Client       *http.Client
	RecordMap    map[string]*Record
	AliasMap     map[string]string
}

func getLogLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo // default if env var not set or unrecognized
	}
}

var DefaultLogger *slog.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level:     getLogLevel(),
	AddSource: true,
}))

func NewStore() *Store {
	return &Store{
		Logger:       DefaultLogger,
		Lock:         &sync.RWMutex{},
		PingInterval: time.Second * 5,
		Client:       &http.Client{},
		RecordMap:    map[string]*Record{},
		AliasMap:     map[string]string{},
	}
}

func (s *Store) Mut(m func(*Store)) {
	s.Lock.Lock()
	m(s)
	s.Lock.Unlock()
	s.OnUpdate()
}

func (s *Store) OnUpdate() {
	if s.OnUpdateFunc == nil {
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
	s.Mut(func(store *Store) {
		store.AliasMap[k] = v
	})
}

func (s *Store) Unalias(k string) {
	s.Mut(func(store *Store) {
		delete(store.AliasMap, k)
	})
}

func (s *Store) Aliases() (all map[string]string) {
	s.Lock.RLock()
	all = maps.Clone(s.AliasMap)
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

func (s *Store) RemoveSession(tssn tunnel.Session) {
	s.Mut(func(store *Store) {
		for _, rec := range store.RecordMap {
			if rec.Session == tssn {
				delete(store.RecordMap, rec.Key)
				s.Logger.Debug("remove", "key", rec.Key)
				break
			}
		}
	})
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

	var has bool
	s.Mut(func(store *Store) {
		_, has = store.RecordMap[k]
		store.RecordMap[k] = rec
	})

	var action string
	if has {
		action = "update"
	} else {
		action = "insert"
	}
	s.Logger.Debug(action, "key", rec.Key, "ip", rec.IP)

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
		s.Logger.Warn(fmt.Sprintf("stm0: unknown command: %s", line))
	}
	s.RemoveSession(r.Session)
}

func (s *Store) Subscribe(upgrader edge.Upgrader) {
	for {
		r, err := upgrader.Upgrade()
		if err == io.EOF {
			s.Logger.Warn("upgrade EOF")
			break
		}

		if err != nil {
			s.Logger.Warn(fmt.Sprintf("upgrade session failed: %s", err))
			continue
		}

		s.Logger.Debug("subscribe", "request", r)

		key, err := s.Allocate(r)
		if err != nil {
			s.Logger.Warn(fmt.Sprintf("allocate resource failed: %s", err))
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
