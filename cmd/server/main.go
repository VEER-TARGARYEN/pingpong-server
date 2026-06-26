package main

import (
	"log"
	"net/http"
	"os"

	"github.com/VEER-TARGARYEN/pingpong-server/internal/network"
)

func main() {
	// The Hub runs in its own goroutine and owns all matchmaking.
	hub := network.NewHub()
	go hub.Run()

	// /ws is where a client "upgrades" from plain HTTP to a WebSocket.
	http.HandleFunc("/ws", hub.ServeWS)

	// A trivial health endpoint. Render (Phase 5) pings this to know we're alive.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Render injects the port via $PORT. Locally we fall back to 8080.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("ping-pong server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server died: %v", err)
	}
}
