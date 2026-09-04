PYTHON ?= python3

.PHONY: init build up down logs check smoke persistence
init:
	$(PYTHON) scripts/setup_env.py
build:
	docker compose build
up: init
	docker compose up -d --build --wait --wait-timeout 240
down:
	docker compose down
logs:
	docker compose logs --tail=100 -f
check:
	go build ./...
	go vet ./...
smoke:
	$(PYTHON) tests/smoke.py --restart-processor
persistence:
	$(PYTHON) tests/persistence.py
