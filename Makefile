BINARY  := httpcron
CLIENT  := httpclient
CMD     := ./cmd/server
CLICMD     := ./cmd/http-server
BIN_DIR := bin

DB_PATH          ?= httpcron.db
ADDR             ?= :8080
REGISTRATION_KEY ?= dev-registration-key

.PHONY: build run test generate install-tools clean lint

## build: compile the server binary to bin/httpcron
build:
	@mkdir -p $(BIN_DIR)
	( cd ./cmd/server ; ../../bin/generate-git-commit.sh )
	go build -o $(BIN_DIR)/$(BINARY) $(CMD)
	go build -o $(BIN_DIR)/$(CLIENT) $(CMD)

## run: run the server (set REGISTRATION_KEY env var for non-dev use)
run:
	REGISTRATION_KEY=$(REGISTRATION_KEY) DB_PATH=$(DB_PATH) ADDR=$(ADDR) \
		go run $(CMD)

## test: run all integration tests
test:
	go test ./tests/... -v -count=1

## generate: regenerate server stubs from openapi.yaml using oapi-codegen
generate: install-tools
	oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml > api/api.gen.go

## install-tools: install required code-generation tools
install-tools:
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

## tidy: tidy and verify module dependencies
tidy:
	go mod tidy

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## clean: remove build artefacts and database
clean:
	rm -rf $(BIN_DIR) $(DB_PATH)

# git push origin v1.0.0
git_set_tag:
	git tag v0.0.6
	git push origin --tags

.DEFAULT_GOAL := build
