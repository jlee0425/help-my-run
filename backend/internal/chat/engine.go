package chat

import (
	"context"
	"encoding/json"
	"time"

	"help-my-run/backend/internal/store"
)

// chatInput is the JSON piped to claude -p stdin: the deterministic pack, the
// last-N conversation turns (oldest-first), and the new user question.
type chatInput struct {
	Pack    ChatContextPack     `json:"pack"`
	History []store.ChatMessage `json:"history"` // last-N, oldest-first
	Message string              `json:"message"` // the new user question
}

// chatArgs builds the claude -p argv (verbatim mirror of progress.analyzeArgs).
func (e *Engine) chatArgs() []string {
	return []string{
		"-p", chatPrompt,
		"--model", e.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}

// Answer persists the user turn, builds the pack + last-N history, calls
// claude -p, persists + returns the assistant turn.
//   - setup error (store/marshal/profile-missing) -> that error
//   - llm.Call error (*llm.CallError | llm.ErrMalformedJSON) -> returned verbatim
//     (handler maps to 502); NO assistant turn is persisted, NO fabrication.
func (e *Engine) Answer(ctx context.Context, message string) (store.ChatMessage, error) {
	// 1) Persist the user turn first.
	if _, err := e.store.AppendChatMessage("user", message); err != nil {
		return store.ChatMessage{}, err
	}

	// 2) Build the deterministic context pack (setup error -> return).
	pack, err := e.buildContextPack(ctx)
	if err != nil {
		return store.ChatMessage{}, err
	}

	// 3) Last-N history (oldest-first; includes the just-appended user turn).
	hist, err := e.store.ListChatMessages(e.historyTurns)
	if err != nil {
		return store.ChatMessage{}, err
	}

	// 4) Marshal the stdin payload (setup error -> return).
	inputJSON, err := json.Marshal(chatInput{Pack: pack, History: hist, Message: message})
	if err != nil {
		return store.ChatMessage{}, err
	}

	// 5) Call claude -p. On ANY llm.Call failure, return the typed error verbatim
	//    (no fallback, no fabrication, no assistant turn persisted).
	var parsed struct {
		Text string `json:"text"`
	}
	if cerr := e.llm.Call(ctx, e.chatArgs(), string(inputJSON), &parsed); cerr != nil {
		return store.ChatMessage{}, cerr
	}

	// 6) Persist + return the assistant turn.
	id, err := e.store.AppendChatMessage("assistant", parsed.Text)
	if err != nil {
		return store.ChatMessage{}, err
	}
	return store.ChatMessage{
		ID:        id,
		Role:      "assistant",
		Content:   parsed.Text,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
