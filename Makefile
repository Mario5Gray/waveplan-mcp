.PHONY: build install test clean

BINARY_NAME := waveplan-mcp
INSTALL_DIR  := $(HOME)/.local/bin
SHARE_DIR    := $(HOME)/.local/share/waveplan
GIT_SHA      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS := -X main.gitSha=$(GIT_SHA)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)

install: build
	@mkdir -p $(INSTALL_DIR)
	install -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"
	@mkdir -p $(SHARE_DIR)/plans
	@if [ ! -d $(SHARE_DIR)/plans ]; then \
		echo "Created data directory $(SHARE_DIR)/plans"; \
	else \
		echo "Skipping $(SHARE_DIR)/plans - already exists"; \
	fi

test:
	go test -v ./...

clean:
	rm -f $(BINARY_NAME)
