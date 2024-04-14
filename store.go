package relay

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

func (sm *SessionStore) Records() (all []*Record) {
	all = maps.Values(sm.Record)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Since.After(all[j].Since)
	})
	return
}

func (sm *SessionStore) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	all := sm.Records()
	resp, err := tags.UnescapedJSONMarshalIndent(all, "  ")
	if err != nil {
		slog.Warn(fmt.Sprintf("json marshal failed: %s", err))
		return
	}
	w.Write(resp)
}

func (sm *SessionStore) Visited(k string) {
	sm.Lock.Lock()
	rec, ok := sm.Record[k]
	if ok {
		rec.Visited += 1
	}
	sm.Lock.Unlock()
}

func (sm *SessionStore) Remove(k string) {
	sm.Lock.Lock()
	delete(sm.Record, k)
	sm.Lock.Unlock()
	emsg := fmt.Sprintf("Recycled %s", k)
	slog.Info(emsg)
	expvars.WebteleportRelaySessionsClosed.Add(1)
}

func (sm *SessionStore) Get(k string) (transport.Session, bool) {
	k, _ = idna.ToASCII(k)
	host, _, _ := strings.Cut(k, ":")
	sm.Lock.RLock()
	// ssn, ok := sm.Sessions[host]
	rec, ok := sm.Record[host]
	sm.Lock.RUnlock()
	if ok {
		return rec.Session, true
	}
	return nil, false
}

func (sm *SessionStore) Add(k string, tssn transport.Session, tstm transport.Stream, vals url.Values) {
	k, _ = idna.ToASCII(k)
	sm.Lock.Lock()

	since := time.Now()
	tags := tags.Tags{Values: vals}
	rec := &Record{
		Session: tssn,
		Tags:    tags,
		Since:   since,
		Visited: 0,
		Key:     k,
	}
	sm.Record[k] = rec

	sm.Lock.Unlock()

	go sm.Ping(k, tstm)
	go sm.Scan(k, tstm)

	expvars.WebteleportRelaySessionsAccepted.Add(1)
}

func (sm *SessionStore) Ping(k string, tstm transport.Stream) {
	for {
		time.Sleep(sm.PingInterval)
		_, err := io.WriteString(tstm, fmt.Sprintf("%s\n", "PING"))
		if err != nil {
			break
		}
	}
	sm.Remove(k)
}

func (sm *SessionStore) Scan(k string, tstm transport.Stream) {
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
			sm.Remove(k)
			break
		}
		slog.Warn(fmt.Sprintf("stm0: unknown command: %s", line))
	}
}

func (sm *SessionStore) Allocate(r *http.Request, root string) (string, string, error) {
	var (
		candidates = utils.ParseDomainCandidates(r.URL.Path)
		Values     = r.URL.Query()
		clobber    = Values.Get("clobber")
	)

	sub := ""
	pickRandom := len(candidates) == 0

	// Try to lease the first available subdomain if candidates are provided
	for _, pfx := range candidates {
		k := fmt.Sprintf("%s.%s", pfx, root)
		rec, exist := sm.Record[k]
		if !exist || (clobber != "" && rec.Tags.Get("clobber") == clobber) {
			sub = pfx
			break
		}
	}

	if sub == "" && !pickRandom {
		return "", "", fmt.Errorf("none of your requested subdomains are currently available: %v", candidates)
	}

	sub = rng.NewDockerSepDigits("-", 4)

	hostname := fmt.Sprintf("%s.%s", sub, root)
	hostnamePath := fmt.Sprintf("%s/%s/", root, sub)
	key := hostname

	if strings.HasSuffix(r.URL.Path, "/") && r.URL.Path != "/" {
		return key, hostnamePath, nil
	}
	return key, hostname, nil
}

func (sm *SessionStore) Negotiate(r *http.Request, root string, tssn transport.Session, tstm transport.Stream) (string, error) {
	key, hp, err := sm.Allocate(r, root)
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
