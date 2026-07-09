# Load .env so targets can read PORT etc.
-include .env
export

.PHONY: run-backend run-web build build-web garmin-login sync test

run-backend:
	cd backend && go run ./cmd/server

# Frontend dev server with /api proxied to the Go backend on :8080.
run-web:
	cd web && npm run dev

# Build the SPA and copy it where go:embed picks it up.
build-web:
	cd web && npm install && npm run build
	rm -rf backend/internal/webui/dist && mkdir -p backend/internal/webui/dist
	cp -r web/dist/. backend/internal/webui/dist/
	touch backend/internal/webui/dist/.keep

# Single self-hosting binary: UI embedded in the Go server.
build: build-web
	mkdir -p bin
	cd backend && go build -o ../bin/helpmyrun ./cmd/server

# Optional CLI Garmin login (the web UI onboarding is the primary path).
garmin-login:
	cd garmin-worker && . .venv/bin/activate && python worker.py login

# Trigger a sync against the running backend. Export API_TOKEN from
# Settings -> Security first (the token is shown once when generated).
sync:
	curl -fsS -X POST -H "Authorization: Bearer $(API_TOKEN)" http://localhost:$(PORT)/api/sync
	@echo

# Run all three test suites: Go core, Python worker, web app.
test:
	cd backend && go test ./...
	cd garmin-worker && . .venv/bin/activate && python -m pytest tests -q
	cd web && npm test
