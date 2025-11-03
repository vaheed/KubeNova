SHELL := /bin/bash
.ONESHELL:

KIND_CLUSTER ?= kubenova-e2e
E2E_KIND_CLUSTER ?= $(KIND_CLUSTER)

dev-up:
	docker compose -f docker-compose.dev.yml up -d --build

dev-down:
	docker compose -f docker-compose.dev.yml down -v

kind-up:
	kind create cluster --name $(KIND_CLUSTER) --config kind/kind-config.yaml
	kubectl cluster-info

platform-up:
	@echo "[platform-up] Add-ons are bootstrapped by the Agent; nothing to do."

deploy-manager:
	docker compose -f docker-compose.dev.yml up -d --build

deploy-agent:
	@echo "[deploy-agent] Not required; Manager installs Agent automatically upon cluster registration."

test-unit:
	go test ./... -count=1

test-e2e:
	E2E_KIND_CLUSTER=$(E2E_KIND_CLUSTER) go test ./tests/e2e/... -count=1 -timeout=45m

kind-flow:
	bash kind/scripts/run_user_flow.sh

manager-up:
	docker compose -f docker-compose.dev.yml up -d --build manager db

agent-build:
	docker build -t ghcr.io/vaheed/kubenova-agent:dev -f build/Dockerfile.agent .

agent-push:
	docker push ghcr.io/vaheed/kubenova-agent:dev

docs-build:
	cd docs/site && npm ci || npm install
	cd docs/site && npm run docs:build

docs-serve:
	cd docs/site && npm run docs:serve

down:
	kind delete cluster --name $(KIND_CLUSTER)
