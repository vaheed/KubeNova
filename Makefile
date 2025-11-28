.PHONY: fmt vet test build docs-dev docs-build compose-up compose-down kind-up e2e-live

KUBENOVA_E2E_BASE_URL ?= http://localhost:8080
KUBENOVA_E2E_KUBECONFIG ?= kind/config

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./... -count=1

build:
	go build ./...

docs-dev:
	npm run docs:dev

docs-build:
	npm run docs:build

compose-up:
	docker compose -f docker-compose.dev.yml up -d db manager

compose-down:
	docker compose -f docker-compose.dev.yml down

kind-up:
	./kind/e2e.sh

e2e-live:
	RUN_LIVE_E2E=1 \
	KUBENOVA_E2E_BASE_URL=$(KUBENOVA_E2E_BASE_URL) \
	KUBENOVA_E2E_KUBECONFIG=$(KUBENOVA_E2E_KUBECONFIG) \
	go test -tags=integration ./internal/manager -run LiveAPIE2E -count=1 -v
