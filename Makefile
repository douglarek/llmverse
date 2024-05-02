GOARCH ?= $(shell go env GOARCH)
BUILD_ARGS := -trimpath -ldflags "-s -w" $(BUILD_ARGS)
OUTPUT ?= llmverse
GOOS ?= $(shell go env GOOS)

.PHONY: llmverse

ifndef CGO_ENABLED
llmverse: export CGO_ENABLED=0
endif
llmverse:
	go build -o $(OUTPUT) $(BUILD_ARGS) cmd/bot/main.go