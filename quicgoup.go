package relay

import (
	"context"

	"github.com/quic-go/quic-go"
	"github.com/webteleport/utils"
	qg "github.com/webteleport/webteleport/transport/quic-go"
)

type QuicGoUpgrader struct {
	*quic.Listener
	HOST string
}

func (s *QuicGoUpgrader) Host() string {
	return s.HOST
}

func (s *QuicGoUpgrader) Upgrade() (*Request, error) {
	conn, err := s.Listener.Accept(context.Background())
	if err != nil {
		return nil, err
	}

	tssn := &qg.QuicSession{conn}

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
