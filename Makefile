.PHONY: build
build:
	@go build -o bin/mrs ./cmd/api

.PHONY: run
run: build
	@./bin/mrs

.PHONY: test
test:
	@go test -race -v ./...