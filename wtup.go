package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	wt "github.com/quic-go/webtransport-go"
	"github.com/webteleport/webteleport/transport"
	"github.com/webteleport/webteleport/transport/webtransport"
)

type WebtransportUpgrader struct {
	root string
	*wt.Server
}

func (s *WebtransportUpgrader) Root() string {
	return s.root
}

func (s *WebtransportUpgrader) IsRoot(r *http.Request) (result bool) {
	origin, _, _ := strings.Cut(r.Host, ":")
	return origin == s.Root()
}

func (s *WebtransportUpgrader) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-webtransport-upgrade") != "" && s.IsRoot(r)
}

func (s *WebtransportUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (transport.Session, transport.Stream, error) {
	ssn, err := s.Server.Upgrade(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("webtransport upgrade failed: %s", err))
		w.WriteHeader(500)
		return nil, nil, err
	}

	tssn := &webtransport.WebtransportSession{ssn}
	tstm, err := tssn.OpenStream(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("webtransport stm0 init failed: %s", err))
		return nil, nil, err
	}

	return tssn, tstm, nil
}
