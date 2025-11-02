SHELL := /bin/bash
.ONESHELL:

KIND_CLUSTER ?= kubenova-e2e

dev-up:
	docker compose -f docker-compose.dev.yml up -d --build

dev-down:
	docker compose -f docker-compose.dev.yml down -v

kind-up:
	kind create cluster --name $(KIND_CLUSTER) --config kind/kind-config.yaml
	kubectl cluster-info

platform-up:
	bash kind/scripts/install_capsule.sh
	bash kind/scripts/install_capsule_proxy.sh
	bash kind/scripts/install_kubevela.sh

deploy-api:
	bash kind/scripts/deploy_kubenova_api.sh

deploy-agent:
	bash kind/scripts/deploy_kubenova_agent.sh

test-smoke:
	bash kind/tests/smoke.sh

test-unit:
	go test ./... -count=1

test-e2e: kind-up platform-up deploy-api
	$(MAKE) test-smoke

manager-up:
	docker compose -f docker-compose.dev.yml up -d --build api db

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
