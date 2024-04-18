package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/btwiuse/wsconn"
	"github.com/hashicorp/yamux"
	"github.com/webteleport/webteleport/transport"
	"github.com/webteleport/webteleport/transport/websocket"
)

type WebsocketUpgrader struct {
	root string
}

func (s *WebsocketUpgrader) Root() string {
	return s.root
}

func (s *WebsocketUpgrader) IsRoot(r *http.Request) (result bool) {
	origin, _, _ := strings.Cut(r.Host, ":")
	return origin == s.Root()
}

func (s *WebsocketUpgrader) IsUpgrade(r *http.Request) (result bool) {
	return r.URL.Query().Get("x-websocket-upgrade") != "" && s.IsRoot(r)
}

func YamuxReverseGender(conn io.ReadWriteCloser, config *yamux.Config, r *http.Request) (string, *yamux.Session, error) {
	// for compatibility with old clients
	// by default, assume opposite side is client
	// TODO over time, we will drop this compatibility
	// and assume opposite side is always server
	if r.Header.Get("Yamux") == "" && r.URL.Query().Get("yamux") == "" {
		ssn, err := yamux.Server(conn, config)
		return "server", ssn, err
	}
	// default gender of new clients is server
	ssn, err := yamux.Client(conn, config)
	return "client", ssn, err
}

func (*WebsocketUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (tssn transport.Session, tstm transport.Stream, err error) {
	conn, err := wsconn.Wrconn(w, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("websocket upgrade failed: %s", err))
		w.WriteHeader(500)
		return
	}
	config := websocket.YamuxConfig(io.Discard)
	gender, ssn, err := YamuxReverseGender(conn, config, r)
	if err != nil {
		slog.Warn(fmt.Sprintf("websocket creating yamux %s failed: %s", gender, err))
		w.WriteHeader(500)
		return
	}
	tssn = &websocket.WebsocketSession{Session: ssn}
	tstm, err = tssn.OpenStream(context.Background())
	if err != nil {
		slog.Warn(fmt.Sprintf("websocket stm0 init failed: %s", err))
		return
	}
	return
}
