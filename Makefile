.PHONY: build install test clean

BINARY_NAME := waveplan-mcp
INSTALL_DIR  := $(HOME)/.local/bin
SHARE_DIR    := $(HOME)/.local/share/waveplan

build:
	go build -o $(BINARY_NAME)

install: build
	@mkdir -p $(INSTALL_DIR)
	install -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"
	@mkdir -p $(SHARE_DIR)/plans
	@echo "Created data directory $(SHARE_DIR)/plans"

test:
	go test -v ./...

clean:
	rm -f $(BINARY_NAME)