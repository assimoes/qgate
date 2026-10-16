#!/usr/bin/env bash
# The one command that decides whether a change is ready: formatting, module
# tidiness, lint (staticcheck included) and the race-enabled test suite. The
# pre-commit hook and CI both run it.
set -euo pipefail
cd "$(dirname "$0")/.."

GOLANGCI_LINT_VERSION=v2.13.2

step() { printf '\n== %s\n' "$1"; }

step "go mod tidy"
go mod tidy -diff

step "golangci-lint (formatting, vet, staticcheck, errorlint, revive, ...)"
go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}" run ./...

step "go test -race"
go test -race ./...

printf '\nall checks passed\n'
