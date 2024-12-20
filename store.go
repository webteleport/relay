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
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/edge"
	"github.com/webteleport/webteleport/tunnel"
	"golang.org/x/exp/maps"
	"golang.org/x/net/idna"
)

var _ Storage = (*Store)(nil)

var DefaultStorage = NewStore()

type Store struct {
	Lock         *sync.RWMutex
	PingInterval time.Duration
	Verbose      bool
	Webhook      string
	Client       *http.Client
	Record       map[string]*Record
}

func NewStore() *Store {
	return &Store{
		Lock:         &sync.RWMutex{},
		PingInterval: time.Second * 5,
		Verbose:      os.Getenv("VERBOSE") != "",
		Webhook:      os.Getenv("WEBHOOK"),
		Client:       &http.Client{},
		Record:       map[string]*Record{},
	}
}

func (s *Store) Records() (all []*Record) {
	s.Lock.RLock()
	all = maps.Values(s.Record)
	s.Lock.RUnlock()
	sort.Slice(all, func(i, j int) bool {
		return all[i].Since.After(all[j].Since)
	})
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

func (s *Store) Visited(k string) {
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

func (s *Store) RemoveSession(tssn tunnel.Session) {
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

func (s *Store) GetSession(h string) (tunnel.Session, bool) {
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

func (s *Store) Allocate(r *edge.Edge) (string, error) {
	k := deriveOnionID(r.Path)
	_, err := io.WriteString(r.Stream, fmt.Sprintf("HOST %s\n", k))
	if err != nil {
		return "", err
	}
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
			slog.Warn(fmt.Sprintf("allocate hostname failed: %s", err))
			continue
		}

		s.Upsert(key, r)
	}
}
