package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/btwiuse/wsconn"
	"github.com/webteleport/transport"
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

func (*WebsocketUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (tssn transport.Session, tstm transport.Stream, err error) {
	conn, err := wsconn.Wrconn(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("websocket upgrade failed: %s", err))
		w.WriteHeader(500)
		return
	}
	gender, ssn, err := websocket.YamuxReverseGender(conn, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("websocket creating yamux %s failed: %s", gender, err))
		w.WriteHeader(500)
		return
	}
	tssn = &websocket.WebsocketSession{Session: ssn}
	tstm, err = tssn.Open(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("websocket stm0 init failed: %s", err))
		return
	}
	return
}
