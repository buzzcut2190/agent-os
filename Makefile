BINARY     := bin/agentfs
BINARY_MCP := bin/agentfs-mcp
VERSION    := 0.5.0
GO         := go
LDFLAGS    := -X main.version=$(VERSION)

.PHONY: build build-mcp test test-mcp test-all lint clean

build: build-mcp
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/agentfs/

build-mcp:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY_MCP) ./cmd/agentfs-mcp/

test:
	$(GO) test ./... -count=1

test-mcp:
	$(GO) test ./pkg/mcp/... -count=1 -timeout 60s

test-all:
	$(GO) test ./... -count=1

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
RELEASE_DIR := release

.PHONY: build-all release install bump-version

build-all:
	@mkdir -p $(RELEASE_DIR)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "Building agentfs for $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch $(GO) build -ldflags "$(LDFLAGS)" -o $(RELEASE_DIR)/agentfs-$$os-$$arch/agentfs ./cmd/agentfs/; \
		echo "Building agentfs-mcp for $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch $(GO) build -ldflags "$(LDFLAGS)" -o $(RELEASE_DIR)/agentfs-mcp-$$os-$$arch/agentfs-mcp ./cmd/agentfs-mcp/; \
	done

release: build-all
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		cd $(RELEASE_DIR) && tar -czf agentfs-$$os-$$arch.tar.gz agentfs-$$os-$$arch agentfs-mcp-$$os-$$arch && \
		sha256sum agentfs-$$os-$$arch.tar.gz > agentfs-$$os-$$arch.tar.gz.sha256; \
	done
	@echo "Release artifacts in $(RELEASE_DIR)/"

install:
	install -m 755 bin/agentfs /usr/local/bin/agentfs
	install -m 755 bin/agentfs-mcp /usr/local/bin/agentfs-mcp

bump-version:
	@test -n "$(VERSION)" || (echo "Usage: make bump-version VERSION=v0.3.0" && exit 1)
	@sed -i 's/^VERSION.*:=.*/VERSION    := $(VERSION)/' Makefile
