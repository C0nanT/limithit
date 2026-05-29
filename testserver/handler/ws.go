package handler

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewWsEchoHandler returns a WebSocket echo handler. maxConns limits concurrent
// connections; 0 means unlimited.
func NewWsEchoHandler(maxConns int) http.HandlerFunc {
	var sem chan struct{}
	if maxConns > 0 {
		sem = make(chan struct{}, maxConns)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if sem != nil {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			default:
				http.Error(w, `{"error":"too many connections"}`, http.StatusServiceUnavailable)
				return
			}
		}
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if err := conn.WriteMessage(mt, msg); err != nil {
				break
			}
		}
	}
}
