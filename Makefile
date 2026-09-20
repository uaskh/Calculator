# Developer entry point for the whole repository. `make help` lists the targets.
# Compatible with GNU Make 3.81 (macOS default) and newer.

SHELL := /bin/bash
.DEFAULT_GOAL := help

BACKEND_DIR  := backend
FRONTEND_DIR := frontend
SERVICE      := api
COVERAGE_DIR := coverage
GO           ?= go
NPM          ?= npm
BACKEND_COVERAGE_MIN ?= 80
DOMAIN_COVERAGE_MIN  ?= 90
BENCH_COUNT          ?= 100
BENCH_MAX_MS         ?= 5
GOVULNCHECK_VERSION  ?= v1.8.0

.PHONY: help setup dev run-backend run-frontend fmt fmt-check lint typecheck \
	test test-backend test-frontend test-e2e coverage build vuln verify \
	docker-build docker-up docker-down clean

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

setup: ## Install dependencies and the Playwright browser
	cd $(BACKEND_DIR) && $(GO) mod download && $(GO) mod verify
	cd $(FRONTEND_DIR) && $(NPM) ci
	cd $(FRONTEND_DIR) && npx playwright install chromium

dev: ## Run backend (:8080) and frontend (:5173) together; Ctrl+C stops both
	@bash scripts/dev.sh

run-backend: ## Run the API on :8080
	cd $(BACKEND_DIR) && LOG_FORMAT=text $(GO) run ./cmd/$(SERVICE)

run-frontend: ## Run the web app on :5173 (proxies /api to :8080)
	cd $(FRONTEND_DIR) && $(NPM) run dev

fmt: ## Format Go and frontend sources
	@cd $(BACKEND_DIR) && if command -v golangci-lint >/dev/null 2>&1; then golangci-lint fmt; else gofmt -s -w .; fi
	cd $(FRONTEND_DIR) && $(NPM) run --silent format

fmt-check: ## Fail when a file is not formatted
	@unformatted="$$(cd $(BACKEND_DIR) && gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "Go files need formatting:"; echo "$$unformatted"; exit 1; fi
	cd $(FRONTEND_DIR) && $(NPM) run --silent format:check

lint: ## Static analysis: golangci-lint and ESLint
	cd $(BACKEND_DIR) && golangci-lint run ./...
	cd $(FRONTEND_DIR) && $(NPM) run --silent lint

typecheck: ## TypeScript project check
	cd $(FRONTEND_DIR) && $(NPM) run --silent typecheck

test: test-backend test-frontend ## Unit, component and API tests

test-backend: ## Go tests with the race detector
	cd $(BACKEND_DIR) && $(GO) test -race -count=1 ./...

test-frontend: ## Vitest unit and component tests
	cd $(FRONTEND_DIR) && $(NPM) test

test-e2e: ## Black-box API tests and Playwright journeys (starts both apps)
	cd $(BACKEND_DIR) && $(GO) test -race -count=1 ./test/e2e/...
	cd $(FRONTEND_DIR) && $(NPM) run test:e2e

coverage: ## Tests with coverage thresholds and the evaluator benchmark; writes coverage/ and docs/coverage.md
	@mkdir -p $(COVERAGE_DIR)/backend docs
	cd $(BACKEND_DIR) && $(GO) test -race -count=1 -covermode=atomic -coverpkg=./... \
		-coverprofile=../$(COVERAGE_DIR)/backend/coverage.out ./...
	cd $(BACKEND_DIR) && $(GO) tool cover -html=../$(COVERAGE_DIR)/backend/coverage.out \
		-o ../$(COVERAGE_DIR)/backend/index.html
	cd $(FRONTEND_DIR) && $(NPM) run --silent test:coverage
	cd $(BACKEND_DIR) && set -o pipefail && $(GO) test -run='^$$' -bench=BenchmarkEvaluate -benchtime=$(BENCH_COUNT)x ./internal/calc/ \
		| tee ../$(COVERAGE_DIR)/backend/bench.txt
	bash scripts/coverage-report.sh $(COVERAGE_DIR) $(BACKEND_COVERAGE_MIN) $(DOMAIN_COVERAGE_MIN) $(BENCH_MAX_MS) > docs/coverage.md
	@grep -E '^\| \*\*Total|^\| Lines|^\*\*Benchmark' docs/coverage.md

build: ## Production builds (backend binary in bin/, frontend in frontend/dist)
	cd $(BACKEND_DIR) && CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o ../bin/$(SERVICE) ./cmd/$(SERVICE)
	cd $(FRONTEND_DIR) && $(NPM) run build

vuln: ## Known-vulnerability scans (govulncheck, npm audit)
	cd $(BACKEND_DIR) && $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...
	cd $(FRONTEND_DIR) && $(NPM) audit --omit=dev --audit-level=high

verify: fmt-check lint typecheck coverage build test-e2e vuln ## Everything CI runs except the container smoke test

docker-build: ## Build the container images
	docker compose build

docker-up: ## Run the stack: web on http://localhost:3000, API on http://localhost:8080
	docker compose up --build --detach --wait

docker-down: ## Stop the stack
	docker compose down --remove-orphans

clean: ## Remove build and test output
	rm -rf bin $(COVERAGE_DIR) $(FRONTEND_DIR)/dist $(FRONTEND_DIR)/playwright-report $(FRONTEND_DIR)/test-results
