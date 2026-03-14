#!/usr/bin/env bash

set -euo pipefail

ENV_FILE="${1:-.env}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Missing env file: ${ENV_FILE}"
  exit 1
fi

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
}

while IFS= read -r raw_line || [[ -n "${raw_line}" ]]; do
  line="$(trim "${raw_line%$'\r'}")"

  if [[ -z "${line}" || "${line}" == \#* ]]; then
    continue
  fi

  if [[ "${line}" != *=* ]]; then
    continue
  fi

  key="$(trim "${line%%=*}")"
  value="${line#*=}"

  if [[ ! "${key}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    echo "Skipping invalid env key: ${key}"
    continue
  fi

  if [[ "${value}" == \"*\" && "${value}" == *\" ]]; then
    value="${value:1:-1}"
  elif [[ "${value}" == \'*\' && "${value}" == *\' ]]; then
    value="${value:1:-1}"
  fi

  export "${key}=${value}"
done < "${ENV_FILE}"
