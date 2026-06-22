package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// chatRequestDTO is the POST /api/chat body.
type chatRequestDTO struct {
	Message string `json:"message"`
}

// chatMessageDTO is one chat turn on the wire (snake_case, mirrors the codebase
// DTO convention). Used as the POST response and each GET history element.
type chatMessageDTO struct {
	Role      string `json:"role"` // "user" | "assistant"
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"` // RFC3339
}

// chatHistoryResp is the GET /api/chat response (oldest-first for rendering).
type chatHistoryResp struct {
	Messages []chatMessageDTO `json:"messages"`
}

// chat handles POST /api/chat: decode {message}, reject empty, call the chat
// engine (which persists both turns), return the assistant turn. On engine
// failure (claude -p) return 502 with the typed error message — no fabrication.
func (h *handlers) chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message required"})
		return
	}
	msg, err := h.d.Chat.Answer(r.Context(), req.Message)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, chatMessageDTO{Role: msg.Role, Content: msg.Content, CreatedAt: msg.CreatedAt})
}

// chatHistory handles GET /api/chat?limit=N: returns up to N recent turns
// oldest-first (for rendering). limit clamped to [1,200], default 50.
func (h *handlers) chatHistory(w http.ResponseWriter, r *http.Request) {
	limit := clampQuery(r, "limit", 50, 1, 200)
	msgs, err := h.d.Store.ListChatMessages(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]chatMessageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, chatMessageDTO{Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt})
	}
	writeJSON(w, http.StatusOK, chatHistoryResp{Messages: out})
}

// clearChat handles DELETE /api/chat: wipes the rolling thread, 204 No Content.
func (h *handlers) clearChat(w http.ResponseWriter, r *http.Request) {
	if err := h.d.Store.ClearChatMessages(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
