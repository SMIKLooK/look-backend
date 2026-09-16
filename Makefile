.PHONY: run build test vet fmt

run:
	go run ./cmd/server

build:
	go build -o bin/look-server ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .
