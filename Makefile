BIN_DIR := bin

.PHONY: craeft gendoc generate all prod clean

all: craeft gendoc

craeft: generate gendoc
	go build -o $(BIN_DIR)/craeft ./cmd/craeft

generate:
	go generate craeft/cmd/craeft

gendoc:
	go build -o $(BIN_DIR)/gendoc ./cmd/gendoc

prod:
	go build -ldflags="-s -w" -trimpath -o $(BIN_DIR)/craeft ./cmd/craeft

clean:
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html
	go clean -testcache


# TESTS
test:
	@echo "Running all tests..."
	go test -v ./...

test-server:
	@echo "Running server tests..."
	go test -v ./internal/server/

test-project:
	@echo "Running project tests..."
	go test -v ./internal/project/


test-coverage:
	@echo "Generating coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...

check: test-race test-coverage
	@echo "All checks passed!"
