BINARY := harness
PREFIX ?= $(shell go env GOPATH)/bin

.PHONY: build install test race vet clean

build:            ## build ./harness in the repo
	go build -o $(BINARY) ./cmd/harness

install:          ## build + place the binary on $PREFIX (default: GOPATH/bin)
	@mkdir -p $(PREFIX)
	go build -o $(PREFIX)/$(BINARY) ./cmd/harness
	@echo "installed $(PREFIX)/$(BINARY)"

test:             ## run the unit suite (no API calls)
	go test ./...

race:             ## run the unit suite under the race detector
	go test -race ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
