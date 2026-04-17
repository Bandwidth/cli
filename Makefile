.PHONY: build test install clean lint snapshot

build:
	go build -o band ./cmd/band

test:
	go test ./... -v

install:
	go build -o "$$(go env GOPATH)/bin/band" ./cmd/band

clean:
	rm -f band

lint:
	golangci-lint run ./...

snapshot:
	goreleaser release --snapshot --clean
