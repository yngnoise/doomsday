package drop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type StockEvent struct {
	Stock  int64  `json:"stock"`
	DropID string `json:"drop_id"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[chan StockEvent]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[chan StockEvent]struct{})}
}

func (h *Hub) subscribe(dropID string) chan StockEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan StockEvent, 16)
	if h.clients[dropID] == nil {
		h.clients[dropID] = make(map[chan StockEvent]struct{})
	}
	h.clients[dropID][ch] = struct{}{}
	return ch
}

func (h *Hub) unsubscribe(dropID string, ch chan StockEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.clients[dropID]; ok {
		delete(clients, ch)
		close(ch)
		if len(clients) == 0 {
			delete(h.clients, dropID)
		}
	}
}

func (h *Hub) Broadcast(dropID string, stock int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	event := StockEvent{Stock: stock, DropID: dropID}
	for ch := range h.clients[dropID] {
		select {
		case ch <- event:
		default:
		}
	}
}

func (h *Hub) ConnectedClients(dropID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[dropID])
}

func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	dropID := r.PathValue("dropID")
	if dropID == "" {
		http.Error(w, "missing drop id", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")

	ch := h.subscribe(dropID)
	defer h.unsubscribe(dropID, ch)

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
