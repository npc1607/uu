.PHONY: build test vet

GOCACHE ?= $(CURDIR)/.cache/go-build
export GOCACHE

build:
	go build -buildvcs=false -o bin/uu ./cmd/uu

test:
	go test ./...

vet:
	go vet ./...
