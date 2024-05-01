package relay

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/webteleport/relay/spec"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/common"
	"github.com/webteleport/webteleport/transport/tcp"
)

var _ spec.Upgrader = (*TcpUpgrader)(nil)

type TcpUpgrader struct {
	net.Listener
	HOST string
}

func (s *TcpUpgrader) Root() string {
	return s.HOST
}

func (s *TcpUpgrader) Upgrade() (*spec.Request, error) {
	conn, err := s.Listener.Accept()
	if err != nil {
		return nil, err
	}

	ssn, err := common.YamuxClient(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("tcp creating yamux client failed: %w", err)
	}

	tssn := &tcp.TcpSession{ssn}

	stm0, err := tssn.Open(context.Background())
	if err != nil {
		return nil, fmt.Errorf("open stm0 error: %w", err)
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
