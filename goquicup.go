package relay

import (
	"context"
	"fmt"
	"net/url"

	"github.com/webteleport/webteleport/spec"
	"github.com/webteleport/utils"
	gq "github.com/webteleport/webteleport/transport/go-quic"
	"github.com/webtransport/quic"
)

var _ spec.Upgrader = (*GoQuicUpgrader)(nil)

type GoQuicUpgrader struct {
	Listener *quic.Endpoint
	HOST     string
}

func (s *GoQuicUpgrader) Root() string {
	return s.HOST
}

func (s *GoQuicUpgrader) Upgrade() (*spec.Request, error) {
	conn, err := s.Listener.Accept(context.Background())
	if err != nil {
		return nil, fmt.Errorf("accept error: %w", err)
	}

	tssn := &gq.QuicSession{conn}

	stm0, err := tssn.Accept(context.Background())
	if err != nil {
		return nil, fmt.Errorf("accept stm0 error: %w", err)
	}

	ruri, err := ReadLine(stm0)
	if err != nil {
		return nil, fmt.Errorf("read request uri error: %w", err)
	}

	u, err := url.ParseRequestURI(ruri)
	if err != nil {
		return nil, fmt.Errorf("parse request uri error: %w", err)
	}

	R := &spec.Request{
		Session: tssn,
		Stream:  stm0,
		Path:    u.Path,
		Values:  u.Query(),
		RealIP:  utils.StripPort(conn.RemoteAddr().String()),
	}
	return R, nil
}
