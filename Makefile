BINARY := wt-cycle
VERSION ?= dev

.PHONY: build test install cross-compile release-local clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/$(BINARY)

test:
	go test ./...

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/$(BINARY)

cross-compile:
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY)-linux-amd64 ./cmd/$(BINARY)
	GOOS=linux GOARCH=arm64 go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY)-linux-arm64 ./cmd/$(BINARY)
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY)-darwin-amd64 ./cmd/$(BINARY)
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY)-darwin-arm64 ./cmd/$(BINARY)

release-local:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY) $(BINARY)-linux-* $(BINARY)-darwin-*
