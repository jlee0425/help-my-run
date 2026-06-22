# help-my-run — Milestone 3.3 Design Spec

**Date:** 2026-06-22
**Status:** Approved (M3.3 detailed; build sequenced AFTER M3.2.1 merges)
**Depends on:** M0 + M1 + M2 + M3.1 + M3.2 (+ M3.2.1, which must merge before M3.3 is built)
**Author:** Brainstormed with Claude Code

---

## 1. Context

M3.3 is the final planned slice of the "AI Training Iceberg" build: **chat-with-your-data**.
The user asks free-form questions about their training and gets a grounded, multi-turn
answer from `claude -p` (the user's Claude subscription — no per-token cost), over a curated
pack of their own data. It reuses the engines built in M1/M3.1/M3.2 rather than adding new
analysis.

**Sequencing:** M3.2.1 (activate the Garmin `.FIT` fallback) is in flight. M3.3's
implementation must be branched off `main` **after M3.2.1 merges**, so its migration number
and any shared-file edits build on the post-M3.2.1 tree. (This spec can be written now; the
plan + build follow the merge.)

## 2. Decisions (locked in brainstorming)

| Decision | Choice |
|---|---|
| Data access | **Curated context pack** — Go assembles a compact, relevant summary and sends it with the question to `claude -p`. The LLM gets **no DB access** (no LLM-written SQL). |
| Conversation | **Multi-turn with rolling history** — each call carries the last N turns + the pack so follow-ups work. |
| Threading | **One rolling thread** (single-user), with a clear-history action. |
| Failure | On `claude -p` failure, surface a clear error — **never fabricate** an answer. |

## 3. Goal & success criteria

**Goal:** Free-form, multi-turn Q&A grounded in the user's own training data.

1. A **Chat screen** (message list + input): ask a question, get an answer; follow-ups carry
   context (multi-turn).
2. Each answer is **grounded in a curated context pack** of real metrics; the coach **states
   when the data is insufficient** instead of guessing.
3. Conversation **history persists**; the last N turns are sent with each call.
4. `claude -p` failure → clear error, **no fabricated answer**; the rest of the app is
   unaffected.
5. A **clear-chat** action wipes the history.

## 4. Components (reuses M1/M3.1/M3.2)

- **Context pack builder** (`backend/internal/chat`) — *deterministic Go*. Assembles a
  compact, **token-bounded** pack from existing engines/stores:
  - profile (HR zones, goal text, run constraints),
  - the **M3.1 progress metrics** (pace-at-fixed-HR, resting HR, HRV, VO2max, weekly
    load, and the decoupling trend),
  - recent **activities** (last ~14, key summary fields),
  - a recent **recovery** summary (sleep/HRV/RHR/Body Battery trend),
  - recent **stream analyses** (time-in-zone / decoupling for the last few runs).
  Pure functions, table-driven tests; no raw streams (summary-level only).
- **Chat engine** (reuse `llm.Client`) — builds the prompt: a system block ("data analyst &
  running coach for RX-CrossFit aerobic development; answer ONLY from the provided data;
  explicitly say when the data is insufficient; no racing advice") + the context pack + the
  last **N** conversation turns + the new user message → `claude -p` → free-form text. On
  any `llm.Call` failure, return a typed error (the handler surfaces it; no fabrication).
- **Chat history store** — a `chat_messages` table; persist every turn, send only the last N
  to the LLM.
- **App** — a **Chat** screen (bubble list + input + send + clear) reached from Home.

## 5. Data model (added; SQLite)
- `chat_messages` — `id` (INTEGER PK AUTOINCREMENT), `role` (TEXT `user`|`assistant`),
  `content` (TEXT), `created_at` (TEXT ISO). Migration = the next number after M3.2.1's
  `00007` (i.e. `00008_*.sql`), confirmed against the tree at plan time.

## 6. API (added; all under bearer auth)
- `POST /api/chat` (body `{ "message": "..." }`) → append the user turn; build the context
  pack + the last N turns; call `claude -p`; append + return the assistant turn
  `{ role, content, created_at }`. On LLM failure → 502-class error.
- `GET /api/chat?limit=N` → recent history (newest-last for rendering).
- `DELETE /api/chat` → clear all `chat_messages`.

## 7. Token discipline
The pack is summary-level and capped (no raw streams); the history sent per call is the last
N turns (default a small N, e.g. 6), while older turns remain stored for display but are not
all re-sent. This keeps each `claude -p` call cheap and within context.

## 8. App
- **Chat** screen: a scrollable message list (user/assistant bubbles), a text input + send
  button, a loading state while the answer is generated, a clear error state on failure, and
  a "clear chat" action. Loads recent history on open via `GET /api/chat`. Home gets a nav
  link to Chat.

## 9. Testing
- **Context pack builder**: table-driven Go tests over fixture activities/recovery/progress/
  stream-analyses — assert contents + that it stays within the token/size bound + graceful
  handling of thin data.
- **Chat engine**: stub `claude -p` runner — assert the prompt contains the system block, the
  pack, the last-N history, and the new message; that the answer is returned; and that an
  `llm.Call` failure yields a typed error (no fabricated text).
- **`chat_messages` store**: temp-DB tests (append, list last N, clear).
- **API**: `httptest` for POST/GET/DELETE incl. auth rejection + the LLM-failure path.
- **App**: jest with a mocked api client — send → render answer, history load on open, clear,
  loading + error states.
- **Manual**: ask a real question and confirm a grounded answer (and a sensible "not enough
  data" when appropriate).

## 10. Risks & mitigations
- **Hallucination / ungrounded answers.** Mitigate: the system prompt restricts answers to
  the provided pack and requires an explicit "insufficient data" response; only summary data
  (already computed by trusted engines) is provided.
- **Token growth over a long chat.** Mitigate: cap the pack; send only the last N turns.
- **`claude -p` unavailable / rate-limited.** Mitigate: typed error surfaced in the chat UI;
  no fabrication; the rest of the app is unaffected.
- **Stale data in answers.** Mitigate: the pack is rebuilt from the live store on every call.

## 11. Out of scope
LLM-written/arbitrary SQL or direct DB access for the model (the riskier "ask your own data"
variant — explicitly rejected), voice input, proactive/unprompted insights, and
multi-conversation threading (one rolling thread only). No race/taper content.
