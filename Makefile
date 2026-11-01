.PHONY: help clean clean-db run run-dev run-headless run-headless-dev run-mock dev-logs install-service service-logs test format format-check vet lint package release-snapshot

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

run-dev: ## Run application with the separate development database and socket
	@go run ./cmd/feedhatch/main.go -dev

run-headless: ## Fetch feeds without the TUI and show logs in the terminal
	@go run ./cmd/feedhatch/main.go -d

run-headless-dev: ## Fetch feeds without the TUI, using the development database and socket
	@go run ./cmd/feedhatch/main.go -d -dev

run-mock: ## Run application with sample data in a temporary database
	@go run ./cmd/feedhatch/main.go -mock

dev-logs: ## Follow the log file written by the TUI in development mode
	@tail -F ~/.cache/feedhatch-dev/feedhatch.log

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
	@test -z "$$(gofmt -l .)" || (gofmt -d . && echo 'Run `make format` to fix' && exit 1)

vet: ## Analyze code for errors
	@go vet ./...

lint: ## Check with linter
	@golangci-lint run ./...

release-snapshot: ## Build all release packages locally into dist/ without publishing
	@go run github.com/goreleaser/goreleaser/v2@v2.18.3 release --snapshot --clean

package: clean format-check vet lint ## Package default with linux
	@GOOS=${OS} GOARCH=${ARCH} go build -a -o bin/${OUTPUT_NAME}/ -ldflags='-s -w' ./...
	@echo 'Packed to folder `bin/${OUTPUT_NAME}/`'
