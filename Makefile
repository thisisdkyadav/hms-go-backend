SHELL := /bin/bash

export GOCACHE := $(CURDIR)/.cache/go-build
export GOMODCACHE := $(CURDIR)/.cache/mod

GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

run:
	./scripts/dev.sh

test:
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	source ./scripts/load-env.sh .env && go test ./...

build:
	mkdir -p bin
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	source ./scripts/load-env.sh .env && GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o ./bin/hms-auth ./cmd/api
