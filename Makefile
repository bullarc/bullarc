.PHONY: build test test-v lint fmt vet clean check verify

build:
	go build -o bin/bullarc ./cmd/bullarc

test:
	go test -race -count=1 ./...

test-v:
	go test -race -count=1 -v ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin/

check: fmt vet test

verify:
	go build ./...
	go test -race -count=1 -run TestSmoke ./...
