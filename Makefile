# Load .env so targets can read PORT etc.
-include .env
export

.PHONY: run-backend run-web build build-web garmin-login sync test install demo

# Relative paths in .env are resolved against the REPO ROOT here (the server
# runs from backend/, which used to break them — the old "absolute paths only"
# footgun). Absolute and ~ paths pass through untouched.
run-backend:
	@abs() { case "$$1" in ""|/*|~*) printf '%s' "$$1";; *) printf '%s' "$(CURDIR)/$$1";; esac; }; \
	export DB_PATH="$$(abs "$${DB_PATH:-helpmyrun.db}")"; \
	export PYTHON_BIN="$$(abs "$${PYTHON_BIN:-garmin-worker/.venv/bin/python}")"; \
	export WORKER_SCRIPT="$$(abs "$${WORKER_SCRIPT:-garmin-worker/worker.py}")"; \
	export IMAGE_DIR="$$(abs "$${IMAGE_DIR:-data/crossfit}")"; \
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

# Install as a user-level systemd service (no root). Builds the binary, drops
# it in ~/.local/bin, installs the unit under ~/.config/systemd/user with the
# repo dir baked in as WorkingDirectory, reloads the user manager, then prints
# the enable one-liners (left to you so nothing is started behind your back).
install: build
	mkdir -p $(HOME)/.local/bin
	# Write then atomically rename: `cp` directly over a running binary fails
	# with "Text file busy", so the rename is what makes `install` safe as an
	# upgrade of a live service (the running process keeps its old inode).
	cp bin/helpmyrun $(HOME)/.local/bin/helpmyrun.new
	mv -f $(HOME)/.local/bin/helpmyrun.new $(HOME)/.local/bin/helpmyrun
	mkdir -p $(HOME)/.config/systemd/user
	sed 's|__WORKINGDIR__|$(CURDIR)|g' deploy/helpmyrun.service > $(HOME)/.config/systemd/user/helpmyrun.service
	systemctl --user daemon-reload
	@echo
	@echo "Installed binary -> $(HOME)/.local/bin/helpmyrun"
	@echo "Installed unit   -> $(HOME)/.config/systemd/user/helpmyrun.service"
	@echo
	@echo "First run — enable it (starts now + on every boot):"
	@echo "    systemctl --user enable --now helpmyrun"
	@echo "    loginctl enable-linger $$USER"
	@echo
	@echo "Upgrading an already-running instance? Pick up the new binary with:"
	@echo "    systemctl --user restart helpmyrun"
	@echo
	@echo "Logs: journalctl --user -u helpmyrun -f"

# Demo mode: no Garmin, no Claude — in-memory DB seeded with synthetic data.
demo: build
	./bin/helpmyrun --demo
