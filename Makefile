.PHONY: help clean run test format format-check vet lint package

# amd64, arm64
ARCH ?= amd64
# linux, windows, darwin (macos)
OS ?= linux
OUTPUT_NAME ?= app-linux
TEST ?= all

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

clean: ## Clean build output and cached files
	@echo 'Clean go files'
	@rm -rf ./bin/
	@go clean

run: ## Run application
	@go run ./cmd/feedhatch/main.go

test: ## Run all tests or a specific one with TEST=./internal/feed
	@if [ "${TEST}" = "all" ]; then \
		echo "Run all tests"; \
		go test ./...; \
	else \
		echo "Run ${TEST} tests"; \
		go test "${TEST}"; \
	fi

format: ## Format code
	@go fmt ./...

format-check: ## Fail if code is not formatted
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

vet: ## Analyze code for errors
	@go vet ./...

lint: ## Check with linter
	@golangci-lint run ./...

package: clean format-check vet lint ## Package default with linux
	@GOOS=${OS} GOARCH=${ARCH} go build -a -o bin/${OUTPUT_NAME}/ -ldflags='-s -w' ./...
	@echo 'Packed to folder `bin/${OUTPUT_NAME}/`'
