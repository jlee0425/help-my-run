# Load .env so targets can read PORT, API_TOKEN, etc.
-include .env
export

.PHONY: run-backend garmin-login sync run-app test

run-backend:
	cd backend && go run ./cmd/server

# One-time interactive Garmin login (MFA-aware). Persists tokens to GARMIN_TOKENSTORE.
garmin-login:
	cd garmin-worker && . .venv/bin/activate && python worker.py login

# Trigger a sync against the running backend (the backend must be running).
sync:
	curl -fsS -X POST -H "Authorization: Bearer $(API_TOKEN)" http://localhost:$(PORT)/api/sync
	@echo

run-app:
	cd app && npx expo start

# Run all three test suites: Go core, Python worker, Expo app.
test:
	cd backend && go test ./...
	cd garmin-worker && . .venv/bin/activate && python -m pytest tests -q
	cd app && npm test -- --watchAll=false
