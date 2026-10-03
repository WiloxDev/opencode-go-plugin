# Makefile for OpenCode Go Plugin (CPAMC)

BINARY_NAME ?= opencode-go-linux-amd64.so
PLUGIN_DIR ?= $(HOME)/.config/cpamc/plugins

.PHONY: all build clean install test

all: build

## Build the shared C library (.so) with optimizations
build:
	@echo "==> Building $(BINARY_NAME)..."
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -buildmode=c-shared -o $(BINARY_NAME) main.go
	@echo "==> Successfully compiled $(BINARY_NAME)"

## Install the plugin into the CPAMC plugin directory
install: build
	@echo "==> Installing plugin to $(PLUGIN_DIR)..."
	@mkdir -p $(PLUGIN_DIR)
	@cp $(BINARY_NAME) $(PLUGIN_DIR)/
	@echo "==> Plugin installed to $(PLUGIN_DIR)/$(BINARY_NAME)"

## Clean generated artifacts
clean:
	@echo "==> Cleaning build artifacts..."
	@rm -f $(BINARY_NAME) $(BINARY_NAME:.so=.h)
	@echo "==> Clean complete."
