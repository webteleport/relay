package relay

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/btwiuse/rng"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport"
	"golang.org/x/net/idna"
)

func NewSessionStore() *SessionStore {
	return &SessionStore{
		counter:  0,
		sessions: map[string]transport.Session{},
		values:   map[string]url.Values{},
		ssnstamp: map[string]time.Time{},
		ssn_cntr: map[string]int{},
		slock:    &sync.RWMutex{},
		interval: time.Second * 5,
	}
}

type SessionStore struct {
	counter  int
	sessions map[string]transport.Session
	values   map[string]url.Values
	ssnstamp map[string]time.Time
	ssn_cntr map[string]int
	slock    *sync.RWMutex
	interval time.Duration
}

func (sm *SessionStore) Remove(ssn transport.Session) {
	sm.slock.Lock()
	for k, v := range sm.sessions {
		if v == ssn {
			sm.counter -= 1
			delete(sm.sessions, k)
			delete(sm.values, k)
			delete(sm.ssnstamp, k)
			delete(sm.ssn_cntr, k)
			emsg := fmt.Sprintf("Recycled %s", k)
			slog.Info(emsg)
		}
	}
	sm.slock.Unlock()
	expvars.WebteleportRelaySessionsClosed.Add(1)
}

func (sm *SessionStore) Get(k string) (transport.Session, bool) {
	k, _ = idna.ToASCII(k)
	host, _, _ := strings.Cut(k, ":")
	sm.slock.RLock()
	ssn, ok := sm.sessions[host]
	sm.slock.RUnlock()
	return ssn, ok
}

func (sm *SessionStore) Add(k string, tssn transport.Session, tstm transport.Stream, vals url.Values) {
	k, _ = idna.ToASCII(k)
	sm.slock.Lock()
	sm.counter += 1
	sm.sessions[k] = tssn
	sm.values[k] = vals
	sm.ssnstamp[k] = time.Now()
	sm.ssn_cntr[k] = 0
	sm.slock.Unlock()

	go sm.Ping(tssn, tstm)
	go sm.Scan(tssn, tstm)

	expvars.WebteleportRelaySessionsAccepted.Add(1)
}

func (sm *SessionStore) Ping(tssn transport.Session, tstm transport.Stream) {
	for {
		time.Sleep(sm.interval)
		_, err := io.WriteString(tstm, fmt.Sprintf("%s\n", "PING"))
		if err != nil {
			break
		}
	}
	sm.Remove(tssn)
}

func (sm *SessionStore) Scan(tssn transport.Session, tstm transport.Stream) {
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
			sm.Remove(tssn)
			break
		}
		slog.Warn(fmt.Sprintf("stm0: unknown command: %s", line))
	}
}

func (sm *SessionStore) Allocate(r *http.Request, root string) (string, string, error) {
	var (
		candidates = utils.ParseDomainCandidates(r.URL.Path)
		values     = r.URL.Query()
		clobber    = values.Get("clobber")
		canClobber = clobber != "" && values.Get("clobber") == clobber
	)

	leaseCandidate := ""
	allowRandom := len(candidates) == 0

	// Try to lease the first available subdomain if candidates are provided
	for _, pfx := range candidates {
		k := fmt.Sprintf("%s.%s", pfx, root)
		if _, exist := sm.Get(k); !exist || canClobber {
			leaseCandidate = pfx
			break
		}
	}

	if !(leaseCandidate == "" && allowRandom) {
		return "", "", fmt.Errorf("none of your requested subdomains are currently available: %v", candidates)
	}

	leaseCandidate = rng.NewDockerSepDigits("-", 4)

	hostname := fmt.Sprintf("%s.%s", leaseCandidate, root)
	hostnamePath := fmt.Sprintf("%s/%s/", root, leaseCandidate)
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
		// Notify the client of the hostnamePath
		_, err1 := io.WriteString(tstm, fmt.Sprintf("HOST %s\n", hp))
		if err1 != nil {
			return "", err1
		}
	}
	return key, nil
}
