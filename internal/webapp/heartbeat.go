package webapp

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	tracker = &heartbeatTracker{
		tabs: make(map[string]time.Time),
	}
	shutdownTimer *time.Timer
	shutdownMu    sync.Mutex
)

type heartbeatTracker struct {
	mu   sync.Mutex
	tabs map[string]time.Time
}

func (h *heartbeatTracker) record(tabID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tabs[tabID] = time.Now()
	cancelShutdown()
}

func (h *heartbeatTracker) remove(tabID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.tabs, tabID)
	h.cleanExpiredLocked()
	if len(h.tabs) == 0 {
		triggerShutdown()
	}
}

func (h *heartbeatTracker) checkActive() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanExpiredLocked()
	if len(h.tabs) == 0 {
		triggerShutdown()
	}
}

func (h *heartbeatTracker) cleanExpiredLocked() {
	now := time.Now()
	for id, last := range h.tabs {
		// Background tabs can be throttled, so we use a 50-second tolerance
		if now.Sub(last) > 50*time.Second {
			delete(h.tabs, id)
		}
	}
}

func triggerShutdown() {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	if shutdownTimer != nil {
		return
	}
	log.Println("Nenhuma aba ativa detectada. Agendando encerramento do ContainerWay Web...")
	shutdownTimer = time.AfterFunc(15*time.Second, func() {
		tracker.mu.Lock()
		tracker.cleanExpiredLocked()
		count := len(tracker.tabs)
		tracker.mu.Unlock()

		if count == 0 {
			log.Println("ContainerWay Web encerrado automaticamente.")
			os.Exit(0)
		} else {
			log.Println("Encerramento cancelado: nova aba detectada.")
			shutdownMu.Lock()
			shutdownTimer = nil
			shutdownMu.Unlock()
		}
	})
}

func cancelShutdown() {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	if shutdownTimer != nil {
		shutdownTimer.Stop()
		shutdownTimer = nil
		log.Println("Encerramento automático cancelado (atividade detectada).")
	}
}

// StartHeartbeatMonitor starts checking active heartbeats periodically.
func StartHeartbeatMonitor() {
	go func() {
		// Initial 25-second tolerance for browser to start and load the page
		time.Sleep(25 * time.Second)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			tracker.checkActive()
		}
	}()
}

// handleHeartbeat handles heartbeat pings from the frontend.
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	var body struct {
		TabID string `json:"tabId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TabID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parâmetros inválidos"})
		return
	}
	tracker.record(body.TabID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleHeartbeatUnload handles tab close notifications.
func (s *Server) handleHeartbeatUnload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	var body struct {
		TabID string `json:"tabId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TabID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parâmetros inválidos"})
		return
	}
	tracker.remove(body.TabID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
