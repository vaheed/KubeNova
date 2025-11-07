SHELL := /bin/bash
.ONESHELL:

dev-up:
	docker compose -f docker-compose.dev.yml up -d --build

dev-down:
	docker compose -f docker-compose.dev.yml down -v

platform-up:
	@echo "[platform-up] Add-ons are bootstrapped by the Agent; nothing to do."

deploy-manager:
	docker compose -f docker-compose.dev.yml up -d --build

deploy-agent:
	@echo "[deploy-agent] Not required; Manager installs Agent automatically upon cluster registration."

.PHONY: test-unit

test-unit:
	go test ./... -count=1

manager-up:
	docker compose -f docker-compose.dev.yml up -d --build manager db

agent-build:
	docker build -t ghcr.io/vaheed/kubenova/agent:dev -f build/Dockerfile.agent .

agent-push:
	docker push ghcr.io/vaheed/kubenova/agent:dev
	
down:
	@echo "Nothing to tear down beyond docker compose; run 'make dev-down' if needed."
