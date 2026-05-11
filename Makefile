.PHONY: build install install-bin install-helpers uninstall-helpers test clean build-mcp install-mcp install-config install-specs

BINARY_NAME := waveplan-mcp
MCP_BINARY  := txtstore-mcp
SWIM_BINS   := swim-next-resolve swim-step swim-run swim-journal swim-validate swim-refine-compile swim-refine-run
INSTALL_DIR  := $(HOME)/.local/bin
CONFIG_DIR   := $(HOME)/.config/waveplan-mcp
SHARE_DIR    := $(HOME)/.local/share/waveplan
GIT_SHA      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
HELPER_SCRIPTS := waveplan-cli wp-task-to-agent.sh wp-plan-to-agent.sh wp-emit-wave-execution.sh

LDFLAGS := -X main.gitSha=$(GIT_SHA)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)

build-mcp:
	go build -o $(MCP_BINARY) ./cmd/txtstore-mcp/
	go build -o txtstore ./cmd/txtstore/

install: install-bin install-helpers install-swim-bins install-specs install-config

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

install-mcp: build-mcp install-config
	@mkdir -p $(INSTALL_DIR)
	install -m 755 $(MCP_BINARY) $(INSTALL_DIR)/$(MCP_BINARY)
	install -m 755 txtstore $(INSTALL_DIR)/txtstore
	@echo "Installed $(MCP_BINARY) and txtstore to $(INSTALL_DIR)/"

install-swim-bins:
	@mkdir -p $(INSTALL_DIR)
	@for bin in $(SWIM_BINS); do \
		go build -o "$(INSTALL_DIR)/$$bin" "./cmd/$$bin/"; \
		echo "Installed $$bin to $(INSTALL_DIR)/$$bin"; \
	done

install-config:
	@mkdir -p $(CONFIG_DIR)
	@if [ ! -f "$(CONFIG_DIR)/waveagents.json" ]; then \
		install -m 644 docs/specs/swim-ops-examples/waveagents.json "$(CONFIG_DIR)/waveagents.json"; \
		echo "Seeded $(CONFIG_DIR)/waveagents.json"; \
	else \
		echo "Skipping $(CONFIG_DIR)/waveagents.json - already exists"; \
	fi

install-specs:
	@mkdir -p $(SHARE_DIR)/specs
	@for schema in docs/specs/swim-schedule-schema-v2.json docs/specs/swim-journal-schema-v1.json; do \
		install -m 644 "$$schema" "$(SHARE_DIR)/specs/$$(basename "$$schema")"; \
		echo "Installed $$(basename "$$schema") to $(SHARE_DIR)/specs/"; \
	done

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
	rm -f $(BINARY_NAME) $(MCP_BINARY) txtstore
