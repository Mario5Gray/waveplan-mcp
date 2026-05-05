.PHONY: build install install-bin install-helpers uninstall-helpers test clean

BINARY_NAME := waveplan-mcp
INSTALL_DIR  := $(HOME)/.local/bin
SHARE_DIR    := $(HOME)/.local/share/waveplan
GIT_SHA      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
HELPER_SCRIPTS := waveplan-cli wp-task-to-agent.sh wp-plan-to-agent.sh wp-emit-wave-execution.sh

LDFLAGS := -X main.gitSha=$(GIT_SHA)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)

install: install-bin install-helpers

install-bin: build
	@mkdir -p $(INSTALL_DIR)
	install -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"
	@mkdir -p $(SHARE_DIR)/plans
	@if [ ! -d $(SHARE_DIR)/plans ]; then \
		echo "Created data directory $(SHARE_DIR)/plans"; \
	else \
		echo "Skipping $(SHARE_DIR)/plans - already exists"; \
	fi

install-helpers:
	@mkdir -p $(INSTALL_DIR)
	@for script in $(HELPER_SCRIPTS); do \
		install -m 755 $$script $(INSTALL_DIR)/$$script; \
		echo "Installed $$script to $(INSTALL_DIR)/$$script"; \
	done

uninstall-helpers:
	@for script in $(HELPER_SCRIPTS); do \
		rm -f $(INSTALL_DIR)/$$script; \
		echo "Removed $(INSTALL_DIR)/$$script"; \
	done

test:
	go test -v ./...

clean:
	rm -f $(BINARY_NAME)
