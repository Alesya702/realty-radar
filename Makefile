# Convenience targets for Linux/macOS (and Windows with make installed).
# Every target is a thin wrapper: the underlying commands are listed in README.md.

VERSION ?= dev
BINARY  := bin/realtyradar

.PHONY: build run demo migrate-up migrate-down migrate-status check test test-integration db-up db-down

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./src/cmd/realtyradar

run:
	go run ./src/cmd/realtyradar serve

demo:
	go run ./src/cmd/realtyradar demo

migrate-up:
	go run ./src/cmd/realtyradar migrate up

migrate-down:
	go run ./src/cmd/realtyradar migrate down

migrate-status:
	go run ./src/cmd/realtyradar migrate status

check:
	bash scripts/check.sh

test:
	go test ./...

test-integration:
	bash scripts/test-integration.sh

db-up:
	docker compose up -d postgres

db-down:
	docker compose down
