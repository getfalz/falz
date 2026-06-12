package server

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/project"
	_ "embed"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed hotreload.js
var hotreloadJS []byte

type MsgType string

const (
	MsgTypeReload   MsgType = "reload"   // full page reload
	MsgTypeNavigate MsgType = "navigate" // navigate to a new URL
	MsgTypeBuilt    MsgType = "built"    // a forced build finished
	MsgTypeError    MsgType = "error"    // a forced build failed

	MsgTypeBuild MsgType = "build" // request a forced build
)

type ServerMsg struct {
	Type    MsgType `json:"type"`
	URL     string  `json:"url,omitempty"`
	Path    string  `json:"path,omitempty"`
	ErrText string  `json:"error,omitempty"`
}

type clientMsg struct {
	Type MsgType `json:"type"`
	Path string  `json:"path,omitempty"`
}

type HotReload struct {
	upgrader websocket.Upgrader
	project  builder

	clients map[*wsClient]struct{}
	mu      sync.Mutex
}

type wsClient struct {
	conn   *websocket.Conn
	send   chan ServerMsg
	closed chan struct{}
}

type builder interface {
	ForceBuild(ctx context.Context, path string) (project.BuildStats, error)
}

func NewHotReload(p builder, mux *http.ServeMux) *HotReload {
	hr := &HotReload{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
		project: p,
		clients: make(map[*wsClient]struct{}),
	}

	mux.HandleFunc("/craeft-hotreload.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(hotreloadJS)
	})

	mux.HandleFunc("/ws", hr.wsHandler)
	return hr
}

const (
	writeTimeout = 5 * time.Second
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
	sendBuf      = 16
)

func (h *HotReload) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.DDebug(log.HotReload).Err(err).Msg("websocket upgrade failed")
		return
	}

	c := &wsClient{
		conn:   conn,
		send:   make(chan ServerMsg, sendBuf),
		closed: make(chan struct{}),
	}

	h.register(c)
	log.IInfo(log.HotReload).Str("remote", r.RemoteAddr).Msg("websocket connected")

	go h.writeLoop(c)
	h.readLoop(r.Context(), c)
}

// readLoop runs in the handler goroutine and processes inbound frames until
// the connection closes. It owns the final cleanup.
func (h *HotReload) readLoop(ctx context.Context, c *wsClient) {
	defer func() {
		h.unregister(c)
		close(c.closed)
		c.conn.Close()
		log.DDebug(log.HotReload).
			Msg("websocket disconnected")
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})

	for {
		if ctx.Err() != nil {
			return
		}

		mt, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived,
			) {
				log.DDebug(log.HotReload).Err(err).Msg("websocket read error")
			}
			return
		}
		if mt != websocket.TextMessage {
			continue
		}

		var msg clientMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.DDebug(log.HotReload).Err(err).Msg("websocket bad message; ignoring")
			continue
		}

		switch msg.Type {
		case MsgTypeBuild:
			go h.handleForceBuild(ctx, c, msg.Path)
		default:
			log.DDebug(log.HotReload).Str("type", string(msg.Type)).Msg("websocket unknown message type")
		}
	}
}

// writeLoop drains the send channel and writes frames to the connection.
func (h *HotReload) writeLoop(c *wsClient) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closed:
			return

		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteJSON(msg); err != nil {
				log.TTrace(log.HotReload).Err(err).Msg("websocket write failed")
				c.conn.Close()
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeTimeout)); err != nil {
				log.TTrace(log.HotReload).Err(err).Msg("websocket ping failed")
				c.conn.Close()
				return
			}
		}
	}
}

func (h *HotReload) register(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *HotReload) unregister(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// snapshot returns a point-in-time slice of connected clients.
func (h *HotReload) snapshot() []*wsClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		out = append(out, c)
	}
	return out
}

// broadcast sends msg to every connected client. Clients whose send buffers
// are full are dropped rather than blocking the broadcast.
func (h *HotReload) broadcast(msg ServerMsg) {
	clients := h.snapshot()

	log.DDebug(log.HotReload).
		Int("clients", len(clients)).
		Str("type", string(msg.Type)).
		Msg("broadcast")

	for _, c := range clients {
		select {
		case c.send <- msg:
		default:
			// buffer full – close the slow client
			log.WWarn(log.HotReload).Msg("websocket send buffer full; dropping client")
			c.conn.Close()
			h.unregister(c)
		}
	}
}

// sendTo delivers msg to a single client only.
func (h *HotReload) sendTo(c *wsClient, msg ServerMsg) {
	select {
	case <-c.closed:
	case c.send <- msg:
	}
}

func (h *HotReload) ReloadPage() {
	h.broadcast(ServerMsg{Type: MsgTypeReload})
}

func (h *HotReload) NavigateTo(url string) {
	h.broadcast(ServerMsg{Type: MsgTypeNavigate, URL: url})
}

func (h *HotReload) handleForceBuild(ctx context.Context, requester *wsClient, sourcePath string) {
	if sourcePath == "" {
		h.sendTo(requester, ServerMsg{
			Type:    MsgTypeError,
			Path:    sourcePath,
			ErrText: "build: path is required",
		})
		return
	}

	log.IInfo(log.HotReload).Str("path", sourcePath).Msg("force build requested via websocket")

	stats, err := h.project.ForceBuild(ctx, sourcePath)
	if err != nil {
		log.EError(log.HotReload).Err(err).Str("path", sourcePath).Msg("force build failed")
		h.sendTo(requester, ServerMsg{
			Type:    MsgTypeError,
			Path:    sourcePath,
			ErrText: err.Error(),
		})
		return
	}

	log.IInfo(log.HotReload).
		Str("path", sourcePath).
		Int64("written", stats.Written).
		Msg("force build done")

	h.sendTo(requester, ServerMsg{
		Type: MsgTypeBuilt,
		Path: sourcePath,
	})
}

// Shutdown closes all WebSocket connections gracefully.
func (h *HotReload) Shutdown(_ context.Context) error {
	h.mu.Lock()
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.clients = make(map[*wsClient]struct{})
	h.mu.Unlock()

	for _, c := range clients {
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutting down"),
			time.Now().Add(writeTimeout),
		)
		_ = c.conn.Close()
	}
	return nil
}
