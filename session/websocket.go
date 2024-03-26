package session

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/hashicorp/yamux"
	"github.com/webteleport/webteleport/transport/websocket"
)

type WebsocketSession struct {
	*yamux.Session
	Controller net.Conn
	Values     url.Values
}

func (ssn *WebsocketSession) GetController() net.Conn {
	return ssn.Controller
}

func (ssn *WebsocketSession) GetValues() url.Values {
	return ssn.Values
}

func (ssn *WebsocketSession) InitController(_ctx context.Context) error {
	if ssn.Controller != nil {
		return nil
	}
	stm0, err := ssn.OpenStream()
	if err != nil {
		return err
	}
	ssn.Controller = stm0
	return nil
}

func (ssn *WebsocketSession) OpenConn(_ctx context.Context) (net.Conn, error) {
	// when there is a timeout, it still panics before MARK
	//
	// ctx, _ = context.WithTimeout(ctx, 3*time.Second)
	//
	// turns out the stream is empty so need to check stream == nil
	stream, err := ssn.OpenStream()
	if err != nil {
		return nil, err
	}
	// once ctx got cancelled, err is nil but stream is empty too
	// add the check to avoid returning empty stream
	if stream == nil {
		return nil, fmt.Errorf("stream is empty")
	}
	// log.Println(`MARK`, stream)
	// MARK
	conn := websocket.NewOpenedConn(stream)
	return conn, nil
}
