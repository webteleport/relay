package relay

import (
	"log"
	"net"
	"os"
)

type VerboseConn struct {
	net.Conn
}

func (c *VerboseConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if os.Getenv("VERBOSE_CONN") != "" {
		log.Println("read", n, string(b[:n]))
	}
	return
}

func (c *VerboseConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if os.Getenv("VERBOSE_CONN") != "" {
		log.Println("write", n, string(b[:n]))
	}
	return
}
