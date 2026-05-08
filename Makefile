.PHONY: all build lint vet test check

PROJ_NAME = external-dns-t-cloud-public-webhook
GO ?= go
GOLANGCI_LINT_VERSION ?= v2.11.4

all: check build

build:
	mkdir -p build/bin
	$(GO) build -o build/bin/$(PROJ_NAME) ./cmd/webhook

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

check: lint vet test
