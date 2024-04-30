package relay

import (
	"context"
	"fmt"
	"net/http"

	"github.com/btwiuse/wsconn"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/transport/common"
	"github.com/webteleport/webteleport/transport/websocket"
)

type WebsocketUpgrader struct {
	root string
}

func (s *WebsocketUpgrader) Root() string {
	return s.root
}

func (s *WebsocketUpgrader) IsRoot(r *http.Request) (result bool) {
	return r.Host == s.Root()
}

func (s *WebsocketUpgrader) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-websocket-upgrade") != "" && s.IsRoot(r)
}

func (*WebsocketUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (*Request, error) {
	conn, err := wsconn.Wrconn(w, r)
	if err != nil {
		w.WriteHeader(500)
		return nil, fmt.Errorf("websocket upgrade failed: %w", err)
	}
	ssn, err := common.YamuxClient(conn)
	if err != nil {
		w.WriteHeader(500)
		return nil, fmt.Errorf("websocket creating yamux client failed: %w", err)
	}
	tssn := &websocket.WebsocketSession{Session: ssn}
	tstm, err := tssn.Open(context.Background())
	if err != nil {
		return nil, fmt.Errorf("websocket stm0 init failed: %w", err)
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
