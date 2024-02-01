package session

import (
	"context"
	"fmt"
	"net"
	"net/url"

	webtransportGo "github.com/quic-go/webtransport-go"
	"github.com/webteleport/webteleport/webtransport"
)

type WebtransportSession struct {
	*webtransportGo.Session
	Controller net.Conn
	Values     url.Values
}

func (ssn *WebtransportSession) GetController() net.Conn {
	return ssn.Controller
}

func (ssn *WebtransportSession) GetValues() url.Values {
	return ssn.Values
}

func (ssn *WebtransportSession) InitController(ctx context.Context) error {
	if ssn.Controller != nil {
		return nil
	}
	stm0, err := ssn.OpenConn(ctx)
	if err != nil {
		return err
	}
	ssn.Controller = stm0
	return nil
}

func (ssn *WebtransportSession) OpenConn(ctx context.Context) (net.Conn, error) {
	// when there is a timeout, it still panics before MARK
	//
	// ctx, _ = context.WithTimeout(ctx, 3*time.Second)
	//
	// turns out the stream is empty so need to check stream == nil
	stream, err := ssn.OpenStreamSync(ctx)
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
	conn := webtransport.NewOpenedConn(stream, ssn.Session)
	return conn, nil
}
