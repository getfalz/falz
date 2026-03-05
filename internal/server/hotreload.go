package server

import (
	"context"
	"craeft/internal/log"
	"craeft/internal/utils"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type HotReloadMsg struct {
	Reload bool   `json:"reload"`
	URL    string `json:"url"`
}

type HotReload struct {
	upgrader       websocket.Upgrader
	wsclients      map[*websocket.Conn]bool
	wsclientsMutex sync.Mutex
}

func (h *HotReload) HotReload() error {
	// s.upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	s.routing.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.DDebug(log.Server).Err(err).Msg("websocket upgrade failed")
			return
		}

		log.IInfo(log.Server).Str("remote", r.RemoteAddr).Msg("websocket connected")

		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		})

		h.wsclientsMutex.Lock()
		h.wsclients[conn] = true
		h.wsclientsMutex.Unlock()

		go func() {
			defer func() {
				h.wsclientsMutex.Lock()
				delete(h.wsclients, conn)
				h.wsclientsMutex.Unlock()
				conn.Close()
				log.DDebug(log.Server).Msg("websocket disconnected")
			}()

			for {
				mt, _, err := conn.ReadMessage()
				if err != nil || mt == websocket.CloseMessage {
					// break
					return
				}
			}
		}()

		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			defer conn.Close()
			for range t.C {
				h.wsclientsMutex.Lock()
				if _, ok := h.wsclients[conn]; !ok {
					h.wsclientsMutex.Unlock()
					return
				}
				h.wsclientsMutex.Unlock()
				_ = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
				log.TTrace(log.Server).Msg("websocket ping")
			}
		}()

	})

	return nil
}

func (h *HotReload) sendToClient(msg HotReloadMsg) {
	h.wsclientsMutex.Lock()
	clients := make([]*websocket.Conn, 0, len(h.wsclients))
	for c := range h.wsclients {
		clients = append(clients, c)
	}
	h.wsclientsMutex.Unlock()

	log.DDebug(log.Server).
		Int("clients", len(clients)).
		Bool("reload", msg.Reload).
		Str("url", msg.URL).
		Msg("broadcast hot-reload")

	for _, c := range clients {
		if err := c.WriteJSON(msg); err != nil {
			_ = c.Close()
			h.wsclientsMutex.Lock()
			delete(h.wsclients, c)
			h.wsclientsMutex.Unlock()
			log.DDebug(log.Server).Err(err).Msg("websocket write failed; client removed")
		}
	}
}

func (h *HotReload) reloadPage() {
	message := HotReloadMsg{Reload: true, URL: ""}
	h.sendToClient(message)
}

// TODO remove server shutdown logic
func (h *HotReload) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var errs []error
	log.IInfo(log.Server).Msg("shutdown begin")

	if err := utils.StopComponent(ctx, "hot-reload", func(context.Context) error {
		h.wsclientsMutex.Lock()
		clients := make([]*websocket.Conn, 0, len(h.wsclients))
		for c := range h.wsclients {
			clients = append(clients, c)
		}
		h.wsclients = make(map[*websocket.Conn]bool)
		h.wsclientsMutex.Unlock()
		for _, c := range clients {
			_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutting down"), time.Now().Add(2*time.Second))
			_ = c.Close()
		}
		return nil
	}); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
