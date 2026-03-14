SHELL := /bin/bash

export GOCACHE := $(CURDIR)/.cache/go-build
export GOMODCACHE := $(CURDIR)/.cache/mod

run:
	./scripts/dev.sh

test:
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	source ./scripts/load-env.sh .env && go test ./...

build:
	mkdir -p bin
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	source ./scripts/load-env.sh .env && go build -o ./bin/hms-auth ./cmd/api
