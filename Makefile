.PHONY: build test

build:
	go build -o ~/bin/regresql ./cmd/regresql

test:
	go test ./...
