#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

if [[ ! -f ".env" ]]; then
  echo "Missing .env in ${ROOT_DIR}"
  exit 1
fi

source "${SCRIPT_DIR}/load-env.sh" ".env"

export GOCACHE="${ROOT_DIR}/.cache/go-build"
export GOMODCACHE="${ROOT_DIR}/.cache/mod"
mkdir -p "${GOCACHE}" "${GOMODCACHE}"

if [[ ! -f "go.sum" ]]; then
  echo "go.sum not found. Running go mod tidy once to download dependencies..."
  go mod tidy
fi

exec go run ./cmd/api
