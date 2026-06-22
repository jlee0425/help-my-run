package store

import "time"

// ChatMessage is one persisted turn of the single rolling chat thread (M3.3).
// The snake_case json tags keep the chat engine's stdin payload (chatInput.History)
// uniformly snake_case alongside the pack/DTO; the api wire uses chatMessageDTO,
// not this struct, so these tags only affect the claude -p prompt serialization.
type ChatMessage struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"` // "user" | "assistant"
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"` // RFC3339 UTC
}

// AppendChatMessage inserts one turn (server-stamped created_at) and returns its
// new id. Mirrors InsertPlan (LastInsertId) + SaveOAuthState (now timestamp).
func (s *Store) AppendChatMessage(role, content string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(
		`INSERT INTO chat_messages (role, content, created_at) VALUES (?, ?, ?)`,
		role, content, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListChatMessages returns up to `limit` most-recent turns in OLDEST-FIRST order
// (newest-last) — the order the chat UI renders and the order the engine sends
// to claude. Query newest-first with LIMIT (so we keep the most recent N), then
// reverse to oldest-first.
func (s *Store) ListChatMessages(limit int) ([]ChatMessage, error) {
	rows, err := s.DB.Query(
		`SELECT id, role, content, created_at FROM chat_messages
		 ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ChatMessage, 0)
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// reverse in place -> oldest-first (newest-last)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ClearChatMessages deletes the entire thread (no-op when empty).
func (s *Store) ClearChatMessages() error {
	_, err := s.DB.Exec(`DELETE FROM chat_messages`)
	return err
}
