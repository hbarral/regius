## Build CLI
build: create_dirs
	@echo "Building CLI for $(UNAME_S)"
	@go mod vendor
	@go build -ldflags "-s -w" -o "$(BINARY_PATH)" ./cmd/cli
	@echo "Build complete!"
	@echo $(PATH_INSTRUCTIONS)

## test: run all tests (both root and cli modules)
test:
	@go test -v ./...
	@cd cli && go test -v ./...

## cover: opens coverage in browser
cover:
	@go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

## coverage: displays test coverage
coverage:
	@go test -cover ./...

UNAME_S := $(shell uname -s)

## Detect OS and set paths accordingly
ifeq ($(OS),Windows_NT)
	HOME_DIR := $(shell echo %USERPROFILE%)
	BINARY_PATH := $(HOME_DIR)\.regius\bin\regius.exe
	MKDIR_CMD := mkdir
	PATH_INSTRUCTIONS := "Add $(HOME_DIR)\.regius\bin to your PATH environment variable"
	else ifeq ($(UNAME_S),Darwin)
	HOME_DIR := $(shell echo $$HOME)
	BINARY_PATH := $(HOME_DIR)/.regius/bin/regius
	MKDIR_CMD := mkdir -p
	PATH_INSTRUCTIONS := "Add export PATH=\$$PATH:\$$HOME/.regius/bin to your ~/.zshrc"
else
	HOME_DIR := $(shell echo $$HOME)
	BINARY_PATH := $(HOME_DIR)/.regius/bin/regius
	MKDIR_CMD := mkdir -p
	PATH_INSTRUCTIONS := "Add export PATH=\$$PATH:\$$HOME/.regius/bin to your ~/.bashrc"
endif

## Create directories if they don't exist
create_dirs:
	$(MKDIR_CMD) "$(dir $(BINARY_PATH))"
	@echo "Binary will be installed to: $(BINARY_PATH)"
	@echo $(PATH_INSTRUCTIONS)

.PHONY: build test cover coverage
