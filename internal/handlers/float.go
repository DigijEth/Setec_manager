package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (h *Handler) FloatRegister(w http.ResponseWriter, r *http.Request) {
	if !h.Config.Float.Enabled {
		writeError(w, http.StatusServiceUnavailable, "Float Mode is disabled")
		return
	}

	var body struct {
		UserAgent string `json:"user_agent"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	// Parse TTL
	ttl, err := time.ParseDuration(h.Config.Float.SessionTTL)
	if err != nil {
		ttl = 24 * time.Hour
	}

	sessionID := uuid.New().String()
	clientIP := r.RemoteAddr

	if err := h.DB.CreateFloatSession(sessionID, 0, clientIP, body.UserAgent, time.Now().Add(ttl)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"session_id": sessionID,
		"expires_in": h.Config.Float.SessionTTL,
	})
}

func (h *Handler) FloatSessions(w http.ResponseWriter, r *http.Request) {
	// Clean expired sessions first
	h.DB.CleanExpiredFloatSessions()

	sessions, err := h.DB.ListFloatSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, sessions)
		return
	}
	h.render(w, "float.html", sessions)
}

func (h *Handler) FloatDisconnect(w http.ResponseWriter, r *http.Request) {
	id := paramStr(r, "id")
	if err := h.DB.DeleteFloatSession(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}
