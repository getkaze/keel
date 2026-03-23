package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/terminal"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: checkWSOrigin,
}

// checkWSOrigin validates the Origin header for WebSocket upgrade requests.
// Allows: same-host origin, localhost/127.0.0.1 on same port, empty origin (non-browser clients).
func checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := u.Hostname()
	requestHost := r.Host
	// Strip port from request host for comparison
	if idx := strings.LastIndex(requestHost, ":"); idx != -1 {
		requestHost = requestHost[:idx]
	}
	if originHost == requestHost {
		return true
	}
	if originHost == "localhost" || originHost == "127.0.0.1" || originHost == "::1" {
		return true
	}
	return false
}

// controlMessage is a JSON control frame for terminal resize.
type controlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// TerminalHandler handles GET /ws/terminal — WebSocket PTY bridge.
type TerminalHandler struct{}

func (h *TerminalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serveTerminalWS(w, r, func() (*terminal.Session, error) {
		return terminal.NewSession()
	})
}

// ExecTerminalHandler handles GET /ws/terminal/exec/{name} — docker exec PTY bridge.
type ExecTerminalHandler struct {
	Services *config.ServiceStore
}

func (h *ExecTerminalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "container name required", http.StatusBadRequest)
		return
	}

	// Resolve the actual Docker container name from the service config
	container := "keel-" + name // fallback
	if h.Services != nil {
		if svc, err := h.Services.Get(name); err == nil && svc != nil && svc.Hostname != "" {
			container = svc.Hostname
		}
	}

	serveTerminalWS(w, r, func() (*terminal.Session, error) {
		return terminal.NewExecSession(container)
	})
}

// serveTerminalWS is the shared WebSocket <-> PTY bridge logic.
func serveTerminalWS(w http.ResponseWriter, r *http.Request, newSession func() (*terminal.Session, error)) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("terminal: websocket upgrade: %v", err)
		return
	}
	defer conn.Close()

	sess, err := newSession()
	if err != nil {
		log.Printf("terminal: create session: %v", err)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "failed to create terminal session"))
		return
	}

	// Close session on app context cancellation (server shutdown).
	go func() {
		<-r.Context().Done()
		sess.Close()
	}()

	// Mutex protects concurrent WebSocket writes from the PTY goroutine
	// and the main read loop (which sends close frames).
	var wsMu sync.Mutex
	var wg sync.WaitGroup

	// PTY stdout -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := sess.PTY.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("terminal: pty read: %v", err)
				}
				wsMu.Lock()
				conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "terminal session ended"))
				wsMu.Unlock()
				return
			}
			wsMu.Lock()
			writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			wsMu.Unlock()
			if writeErr != nil {
				return
			}
		}
	}()

	// WebSocket -> PTY stdin (binary) or control (text/JSON)
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("terminal: ws read: %v", err)
			}
			break
		}

		switch msgType {
		case websocket.BinaryMessage:
			if _, err := sess.PTY.Write(data); err != nil {
				log.Printf("terminal: pty write: %v", err)
				break
			}
		case websocket.TextMessage:
			var ctrl controlMessage
			if err := json.Unmarshal(data, &ctrl); err != nil {
				continue
			}
			if ctrl.Type == "resize" && ctrl.Cols > 0 && ctrl.Rows > 0 {
				sess.Resize(ctrl.Rows, ctrl.Cols)
			}
		}
	}

	// Close the session BEFORE waiting for the reader goroutine.
	// This causes PTY.Read() to return io.EOF, unblocking the reader.
	sess.Close()
	wg.Wait()
}
