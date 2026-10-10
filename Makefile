GO ?= go
COMPOSE ?= docker compose

.PHONY: test build up down logs rebuild smoke

test:
	GOTOOLCHAIN=local $(GO) test -mod=vendor ./...

build:
	GOTOOLCHAIN=local $(GO) build -mod=vendor ./cmd/aml-api ./cmd/aml-worker ./cmd/aml-migrate

up:
	$(COMPOSE) up --build

down:
	$(COMPOSE) down --remove-orphans

logs:
	$(COMPOSE) logs --tail=200 -f

rebuild:
	$(COMPOSE) build --no-cache

smoke:
	curl -fsS http://127.0.0.1:$${SENTINEL_AML_HTTP_PORT:-8081}/readyz >/dev/null
