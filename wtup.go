package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	wt "github.com/quic-go/webtransport-go"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/webtransport"
)

type WebtransportUpgrader struct {
	root string
	reqc chan *Request
	*wt.Server
}

func (s *WebtransportUpgrader) Root() string {
	return s.root
}

func (s *WebtransportUpgrader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ssn, err := s.Server.Upgrade(w, r)
	if err != nil {
		slog.Warn(fmt.Errorf("webtransport upgrade failed: %w", err).Error())
	}

	tssn := &webtransport.WebtransportSession{Session: ssn}
	tstm, err := tssn.Open(context.Background())
	if err != nil {
		slog.Warn(fmt.Errorf("webtransport stm0 init failed: %w", err).Error())
	}

	R := &Request{
		Session: tssn,
		Stream:  tstm,
		Path:    r.URL.Path,
		Values:  r.URL.Query(),
		RealIP:  utils.RealIP(r),
	}
	s.reqc <- R
}

func (s *WebtransportUpgrader) Upgrade() (*Request, error) {
	r, ok := <-s.reqc
	if !ok {
		return nil, io.EOF
	}
	return r, nil
}
