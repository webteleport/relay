package relay

import (
	"context"

	"github.com/webteleport/utils"
	gq "github.com/webteleport/webteleport/transport/go-quic"
	"github.com/webtransport/quic"
)

type GoQuicUpgrader struct {
	Listener *quic.Endpoint
	HOST string
}

func (s *GoQuicUpgrader) Host() string {
	return s.HOST
}

func (s *GoQuicUpgrader) Upgrade() (*Request, error) {
	conn, err := s.Listener.Accept(context.Background())
	if err != nil {
		return nil, err
	}

	tssn := &gq.QuicSession{conn}

	stm0, err := tssn.Open(context.Background())
	if err != nil {
		return nil, err
	}

	u, err := readAndParseFirstLine(stm0)
	if err != nil {
		return nil, err
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
