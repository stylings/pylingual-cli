APP := pylingual
OUT_DIR := bin
GOCACHE ?= $(CURDIR)/.cache/go-build
GOFLAGS ?= -buildvcs=false -trimpath

.PHONY: build test race vet check clean

build:
	@mkdir -p $(OUT_DIR)
	GOCACHE=$(GOCACHE) go build $(GOFLAGS) -o $(OUT_DIR)/$(APP) .

test:
	GOCACHE=$(GOCACHE) go test ./...

race:
	GOCACHE=$(GOCACHE) go test -race ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

check: vet test

clean:
	rm -rf $(OUT_DIR) .cache coverage.out
