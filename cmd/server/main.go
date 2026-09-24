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
	defer ln.Close()

	log.Printf("gocache listening on %s", addr)

	s := store.New()

	// Phase 2a: one connection at a time, no concurrency.
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		server.HandleConn(conn, s)
	}
}
