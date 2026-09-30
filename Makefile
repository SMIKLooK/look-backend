run:
	go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -o bin/look-server ./cmd/server

.PHONY: run test vet build
