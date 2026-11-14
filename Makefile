SHELL := /bin/bash

# go.mod's toolchain line is only a minimum; run CI's exact version instead.
GOTOOLCHAIN ?= $(shell sed -n 's/^toolchain //p' go.mod)
export GOTOOLCHAIN
# The oldest Go importers may use: go.mod's go line.
GO_FLOOR := go$(shell sed -n 's/^go //p' go.mod)

GOLANGCI_LINT_VERSION := v2.14.0
BONSAI_LINT_VERSION := v0.5.1
GOVULNCHECK_VERSION := v1.9.0
ACTIONLINT_VERSION := v1.7.12
GITLEAKS_IMAGE := ghcr.io/gitleaks/gitleaks:v8.30.1@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f

FUZZTIME ?= 10m

.PHONY: help fmt lint complexity tidy-check zizmor gitleaks actionlint typos test race floor fuzz cover vuln check

help:
	@echo "fsum make targets:"
	@echo "  fmt         - golangci-lint fmt (gofumpt + goimports, auto-fix)"
	@echo "  lint        - golangci-lint $(GOLANGCI_LINT_VERSION) run"
	@echo "  complexity  - bonsai-lint cognitive complexity gate"
	@echo "  tidy-check  - go.mod matches what go mod tidy produces (changes nothing)"
	@echo "  zizmor      - audit .github/ workflows and dependabot.yml (offline)"
	@echo "  gitleaks    - scan the whole git history for committed secrets"
	@echo "  actionlint  - lint .github/workflows (needs shellcheck)"
	@echo "  typos       - spell-check the repository"
	@echo "  test        - go test ./..."
	@echo "  race        - go test -race ./..."
	@echo "  floor       - go test ./... on go.mod's minimum Go ($(GO_FLOOR))"
	@echo "  fuzz        - deep fuzz run, FUZZTIME=$(FUZZTIME) (release gate)"
	@echo "  cover       - race tests + coverage report (coverage.out)"
	@echo "  vuln        - govulncheck $(GOVULNCHECK_VERSION)"
	@echo "  check       - everything CI runs; green here should mean green in CI"

.DEFAULT_GOAL := help

# Usage: $(call check_tool,<binary>,<install hint>)
define check_tool
@command -v $(1) >/dev/null 2>&1 || { echo "fsum: '$(1)' is required but not installed."; echo "  install: $(2)"; exit 1; }
endef

fmt:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) fmt

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

# Pinned here rather than as go.mod `tool` lines: those need go >= 1.24, which would raise the floor for importers.
complexity:
	go run bonsai.kauneckas.dev/bonsai-lint@$(BONSAI_LINT_VERSION) .

tidy-check:
	go mod tidy -diff

zizmor:
	$(call check_tool,zizmor,brew install zizmor or https://docs.zizmor.sh/installation/)
	zizmor --offline --collect=all --strict-collection .

# --redact keeps a finding's secret out of the public CI log.
gitleaks:
	docker run --rm -v "$(CURDIR):/repo:ro" $(GITLEAKS_IMAGE) git --redact --verbose /repo

# actionlint silently skips its shell-script rules when shellcheck is missing, so require it.
actionlint:
	$(call check_tool,shellcheck,brew install shellcheck or https://github.com/koalaman/shellcheck#installing)
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

typos:
	$(call check_tool,typos,brew install typos-cli or https://github.com/crate-ci/typos#install)
	typos

test:
	go test ./...

race:
	go test -race ./...

floor:
	GOTOOLCHAIN=$(GO_FLOOR) go test ./...

fuzz:
	go test -run '^$$' -fuzz '^FuzzSum$$' -fuzztime $(FUZZTIME) .

cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

check: lint complexity tidy-check zizmor gitleaks actionlint typos race floor vuln
	go vet ./...
