package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/btwiuse/wsconn"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/common"
	"github.com/webteleport/webteleport/transport/websocket"
)

type WebsocketUpgrader struct {
	root string
	reqc chan *Request
}

func (s *WebsocketUpgrader) Root() string {
	return s.root
}

func (s *WebsocketUpgrader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := wsconn.Wrconn(w, r)
	if err != nil {
		w.WriteHeader(500)
		slog.Warn(fmt.Errorf("websocket upgrade failed: %w", err).Error())
		return
	}

	ssn, err := common.YamuxClient(conn)
	if err != nil {
		w.WriteHeader(500)
		slog.Warn(fmt.Errorf("websocket creating yamux client failed: %w", err).Error())
		return
	}

	tssn := &websocket.WebsocketSession{Session: ssn}
	tstm, err := tssn.Open(context.Background())
	if err != nil {
		w.WriteHeader(500)
		slog.Warn(fmt.Errorf("websocket stm0 init failed: %w", err).Error())
		return
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

func (s *WebsocketUpgrader) Upgrade() (*Request, error) {
	r, ok := <-s.reqc
	if !ok {
		return nil, io.EOF
	}
	return r, nil
}
