// cmd/server/main.go
package main

import (
	"log"
	"net"

	"gocache/internal/server"
	"gocache/internal/store"
)

const addr = ":6380"

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}
	log.Printf("gocache listening on %s", addr)

	if err := server.Serve(ln, store.New()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
