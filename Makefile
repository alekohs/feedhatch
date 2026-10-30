.PHONY: help clean clean-db run run-headless run-mock install-service service-logs test format format-check vet lint package

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

clean-db: ## Clean db from cache
	@echo 'Remove db from ~/.cache/feedhatch/*.db'
	@rm -rf ~/.cache/feedhatch/*.db

run: ## Run application
	@go run ./cmd/feedhatch/main.go

run-headless: ## Fetch feeds without the TUI and show logs in the terminal
	@go run ./cmd/feedhatch/main.go -d

run-mock: ## Run application with sample data in a temporary database
	@go run ./cmd/feedhatch/main.go -mock

install-service: ## Install to ~/.local/bin and (re)start the systemd user service
	@go build -o ${HOME}/.local/bin/feedhatch ./cmd/feedhatch
	@mkdir -p ${HOME}/.config/systemd/user
	@cp init/feedhatch.service ${HOME}/.config/systemd/user/
	@systemctl --user daemon-reload
	@systemctl --user enable feedhatch
	@systemctl --user restart feedhatch
	@echo 'Installed. Follow logs with `make service-logs`'

service-logs: ## Follow the systemd user service logs
	@journalctl --user -u feedhatch -f

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
