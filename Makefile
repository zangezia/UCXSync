.PHONY: build run test clean install dev build-riscv64 build-all

APP_NAME=ucxsync
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

build:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(APP_NAME) ./cmd/ucxsync

build-riscv64:
	GOOS=linux GOARCH=riscv64 go build $(LDFLAGS) -o $(APP_NAME)-riscv64 ./cmd/ucxsync

build-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(APP_NAME)-arm64 ./cmd/ucxsync

build-all: build build-riscv64 build-arm64

run:
	go run ./cmd/ucxsync

test:
	go test -v -race ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

clean:
	rm -f $(APP_NAME) $(APP_NAME).exe
	rm -rf bin/ dist/ build/
	rm -f coverage.out

install:
	go install $(LDFLAGS) ./cmd/ucxsync

dev:
	air -c .air.toml

fmt:
	go fmt ./...

lint:
	golangci-lint run

deps:
	go mod download
	go mod tidy

.DEFAULT_GOAL := build
