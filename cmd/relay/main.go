package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/webteleport/relay"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var HOST = getEnv("HOST", "localhost:8080")

func main() {
	store := relay.NewSessionStore()

	tcpUpgrader, err := newTcpUpgrader(HOST, "8081")
	if err != nil {
		log.Fatal(err)
	}
	go serveUpgrader(tcpUpgrader, store)

	s := relay.NewWSServer(HOST, store)
	log.Println("Starting server on http://127.0.0.1:8080")
	http.ListenAndServe(":8080", s)
}

func newTcpUpgrader(host string, port string) (*relay.TcpUpgrader, error) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, err
	}
	log.Println("Starting server on tcp://127.0.0.1:" + port)
	upgrader := &relay.TcpUpgrader{
		Listener: ln,
		HOST:     host,
	}
	return upgrader, nil
}

func serveUpgrader(upgrader relay.Upgrader, s *relay.SessionStore) {
	for {
		R, err := upgrader.Upgrade()
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println(R)

		key, err := s.Negotiate(R, upgrader.Host())
		if err != nil {
			slog.Warn(fmt.Sprintf("negotiate tcp session failed: %s", err))
			return
		}

		log.Println("Negotiated key: ", key)
		s.Upsert(key, R)
	}
}
