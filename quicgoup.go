package relay

import (
	"context"
	"fmt"
	"net/url"

	"github.com/quic-go/quic-go"
	"github.com/webteleport/utils"
	qg "github.com/webteleport/webteleport/transport/quic-go"
)

type QuicGoUpgrader struct {
	*quic.Listener
	HOST string
}

func (s *QuicGoUpgrader) Root() string {
	return s.HOST
}

func (s *QuicGoUpgrader) Upgrade() (*Request, error) {
	conn, err := s.Listener.Accept(context.Background())
	if err != nil {
		return nil, fmt.Errorf("accept error: %w", err)
	}

	tssn := &qg.QuicSession{conn}

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

	R := &Request{
		Session: tssn,
		Stream:  stm0,
		Path:    u.Path,
		Values:  u.Query(),
		RealIP:  utils.StripPort(conn.RemoteAddr().String()),
	}
	return R, nil
}
