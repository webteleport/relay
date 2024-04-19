package main

import (
	"log"
	"net/http"

	"github.com/webteleport/relay"
)

func main() {
	store := relay.NewSessionStore()
	s := relay.NewWSServer("localhost", store)
	log.Println("Starting server on http://127.0.0.1:8080")
	http.ListenAndServe(":8080", s)
}
