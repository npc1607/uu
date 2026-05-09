.PHONY: build test vet

GOCACHE ?= $(CURDIR)/.cache/go-build
export GOCACHE

build:
	go build -buildvcs=false -o uu ./cmd/uu

test:
	go test ./...

vet:
	go vet ./...
