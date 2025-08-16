package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"

	"github.com/webteleport/relay"
)

var TLSConfig = &tls.Config{
	InsecureSkipVerify: true,
	GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		tlsCert, err := tls.LoadX509KeyPair(getEnv("CERT", "cert.pem"), getEnv("KEY", "key.pem"))
		if err != nil {
			return nil, err
		}
		return &tlsCert, nil
	},
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var (
	HOST          = getEnv("HOST", "localhost:8080")
	PORT          = getEnv("PORT", "8080")
	TCP_PORT      = getEnv("TCP_PORT", "8081")
	QUIC_GO_PORT  = getEnv("QUIC_GO_PORT", "8082")
	NET_QUIC_PORT = getEnv("NET_QUIC_PORT", "8083")
	RELAY         = getEnv("RELAY", "https://relay.example.com")
)

func main() {
	log.SetFlags(log.Llongfile)
	os.Setenv("VERBOSE", "1")

	log.Println("HOST:", HOST)

	s := relay.DefaultWSServer(HOST)

	tcpUpgrader, err := newTcpUpgrader(HOST, TCP_PORT)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Starting server on tcp://127.0.0.1:" + TCP_PORT)
	go s.Subscribe(tcpUpgrader)

	quicGoUpgrader, err := newQuicGoUpgrader(HOST, QUIC_GO_PORT)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Starting server on quic-go://127.0.0.1:" + QUIC_GO_PORT)
	go s.Subscribe(quicGoUpgrader)

	goQuicUpgrader, err := newNetQuicUpgrader(HOST, NET_QUIC_PORT)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("[not working yet] Starting server on net-quic://127.0.0.1:" + NET_QUIC_PORT)
	go s.Subscribe(goQuicUpgrader)

	websocketUpgrader, err := newWebsocketUpgrader(HOST, RELAY)
	if err != nil {
		log.Println(err)
	} else {
		log.Println("Starting server on relay:", RELAY)
		go s.Subscribe(websocketUpgrader)
	}

	log.Println("Starting server on http://127.0.0.1:" + PORT)
	http.ListenAndServe(":"+PORT, s)
}
