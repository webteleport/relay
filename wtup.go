package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	wt "github.com/quic-go/webtransport-go"
	"github.com/webteleport/utils"
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
	return utils.StripPort(r.Host) == utils.StripPort(s.Root())
}

func (s *WebtransportUpgrader) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-webtransport-upgrade") != "" && s.IsRoot(r)
}

func (s *WebtransportUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (*Request, error) {
	ssn, err := s.Server.Upgrade(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("webtransport upgrade failed: %s", err))
		w.WriteHeader(500)
		return nil, err
	}

	tssn := &webtransport.WebtransportSession{Session: ssn}
	tstm, err := tssn.Open(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("webtransport stm0 init failed: %s", err))
		return nil, err
	}

	R := &Request{
		Session: tssn,
		Stream:  tstm,
		Path:    r.URL.Path,
		Values:  r.URL.Query(),
		RealIP:  utils.RealIP(r),
	}
	return R, nil
}
