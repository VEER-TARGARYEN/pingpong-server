// Throwaway dev tool: a text-mode player for testing the room-code protocol.
//
// Usage:
//   go run ./cmd/wsclient            -> creates a room, prints the code
//   go run ./cmd/wsclient JOIN CODE  -> joins room CODE
// Once in a match, type: u=up  d=down  s=stop  (then Enter).
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

func main() {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Decide whether to create or join based on command-line args.
	if len(os.Args) >= 3 && strings.EqualFold(os.Args[1], "join") {
		code := strings.ToUpper(os.Args[2])
		send(conn, fmt.Sprintf(`{"t":"join","code":%q}`, code))
		fmt.Printf("joining room %s...\n", code)
	} else {
		send(conn, `{"t":"create"}`)
		fmt.Println("creating room...")
	}

	// Background reader: print control messages in full, throttle state spam.
	go func() {
		count := 0
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("connection closed: %v", err)
				os.Exit(0)
			}
			s := string(msg)
			if strings.Contains(s, `"t":"s"`) {
				count++
				if count%15 == 0 { // ~4/sec
					fmt.Printf("state: %s\n", s)
				}
				continue
			}
			fmt.Printf(">> %s\n", s) // created / start / error / end
		}
	}()

	// Foreground: keystrokes -> input messages.
	fmt.Println("controls: u=up  d=down  s=stop  (Enter to send). Ctrl+C to quit.")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var dir int
		switch strings.TrimSpace(scanner.Text()) {
		case "u":
			dir = -1
		case "d":
			dir = 1
		case "s":
			dir = 0
		default:
			continue
		}
		send(conn, fmt.Sprintf(`{"t":"input","dir":%d}`, dir))
	}
}

func send(conn *websocket.Conn, payload string) {
	if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		log.Printf("write failed: %v", err)
	}
}
