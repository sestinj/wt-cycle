BINARY := wt-cycle
PREFIX := $(HOME)/.local/bin

.PHONY: build test install clean

build:
	go build -o $(BINARY) ./cmd/wt-cycle

test:
	go test ./...

install: build
	mkdir -p $(PREFIX)
	cp $(BINARY) $(PREFIX)/$(BINARY)

clean:
	rm -f $(BINARY)
