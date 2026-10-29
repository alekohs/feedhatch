.PHONY: help clean run package test

ARCH ?= amd64 # amd64,arm64
OS ?= linux #linux, windows, darwin (macos)
OUTPUT_NAME ?= app-linux
TEST ?= all

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

clean: # Clean object- and cached files
	@echo 'Clean go files'
	@go clean

package: clean ## Package default with linux
	@GOOS=${OS} GOARCH=${ARCH} go build -a -o bin/${OUTPUT_NAME}/ -ldflags='-s -w' ./...
	@echo 'Packed to folder `bin/${OUTPUT_NAME}/`'

run: ## Run application
	@go run ./cmd/feedhatch/main.go

test: ## Run all tests or a specific one with TEST=./internal/feed
	@if [ "${TEST}" = "all" ]; then\
			echo "Run all tests" ;\
      go test ./...; \
  fi

	@if [ "${TEST}" != "all" ]; then\
			echo "Run ${TEST} tests" ;\
      go test "${TEST}"; \
  fi
