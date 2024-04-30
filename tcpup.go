package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/common"
	"github.com/webteleport/webteleport/transport/tcp"
)

type TcpUpgrader struct {
	net.Listener
	HOST string
}

func (s *TcpUpgrader) Host() string {
	return s.HOST
}

func (s *TcpUpgrader) Upgrade() (*Request, error) {
	conn, err := s.Listener.Accept()
	if err != nil {
		return nil, err
	}

	ssn, err := common.YamuxClient(conn)
	if err != nil {
		conn.Close()
		slog.Warn(fmt.Sprintf("tcp creating yamux client failed: %s", err))
		return nil, err
	}

	tssn := &tcp.TcpSession{ssn}

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
