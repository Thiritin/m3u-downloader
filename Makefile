.PHONY: build test lint tidy run-tui run-worker

build:
	go build -o m3u-dl ./cmd/m3u-dl

test:
	go test ./...

tidy:
	go mod tidy

lint:
	go vet ./...

run-tui: build
	./m3u-dl tui

run-worker: build
	./m3u-dl worker
