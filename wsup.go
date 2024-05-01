package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/btwiuse/wsconn"
	"github.com/webteleport/webteleport/spec"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/common"
	"github.com/webteleport/webteleport/transport/websocket"
)

var _ spec.HTTPUpgrader = (*WebsocketUpgrader)(nil)

type WebsocketUpgrader struct {
	HOST string
	reqc chan *spec.Request
}

func (s *WebsocketUpgrader) Root() string {
	return s.HOST
}

func (s *WebsocketUpgrader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := wsconn.Wrconn(w, r)
	if err != nil {
		slog.Warn(fmt.Errorf("websocket upgrade failed: %w", err).Error())
		return
	}

	ssn, err := common.YamuxClient(conn)
	if err != nil {
		slog.Warn(fmt.Errorf("websocket creating yamux client failed: %w", err).Error())
		return
	}

	tssn := &websocket.WebsocketSession{Session: ssn}
	tstm, err := tssn.Open(context.Background())
	if err != nil {
		slog.Warn(fmt.Errorf("websocket stm0 init failed: %w", err).Error())
		return
	}

	R := &spec.Request{
		Session: tssn,
		Stream:  tstm,
		Path:    r.URL.Path,
		Values:  r.URL.Query(),
		RealIP:  utils.RealIP(r),
	}
	s.reqc <- R
}

func (s *WebsocketUpgrader) Upgrade() (*spec.Request, error) {
	if s.reqc == nil {
		s.reqc = make(chan *spec.Request, 10)
	}
	r, ok := <-s.reqc
	if !ok {
		return nil, io.EOF
	}
	return r, nil
}
