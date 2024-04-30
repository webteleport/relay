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

var HOST = getEnv("HOST", "localhost:8080")

func main() {
	log.SetFlags(log.Llongfile)
	store := relay.NewSessionStore()

	tcpUpgrader, err := newTcpUpgrader(HOST, "8081")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Starting server on tcp://127.0.0.1:8081")
	go store.Subscribe(tcpUpgrader)

	quicGoUpgrader, err := newQuicGoUpgrader(HOST, "8082")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Starting server on quic-go://127.0.0.1:8082")
	go store.Subscribe(quicGoUpgrader)

	goQuicUpgrader, err := newGoQuicUpgrader(HOST, "8083")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Starting server on go-quic://127.0.0.1:8083 [not working yet]")
	go store.Subscribe(goQuicUpgrader)

	s := relay.NewWSServer(HOST, store)
	log.Println("Starting server on http://127.0.0.1:8080")
	http.ListenAndServe(":8080", s)
}
