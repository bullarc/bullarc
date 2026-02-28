DOCKER_IMAGE ?= bullarc
DOCKER_TAG   ?= latest

.PHONY: build test test-v lint fmt vet clean check verify demo demo-gif docker-build docker-run

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

demo: build
	./bin/bullarc demo

demo-gif: build
	@command -v vhs >/dev/null 2>&1 || { echo "Install VHS: brew install charmbracelet/tap/vhs"; exit 1; }
	PATH="$(CURDIR)/bin:$(PATH)" vhs demo.tape

# Docker targets
docker-build:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Example: make docker-run SYMBOL=AAPL ALPACA_API_KEY=<id> ALPACA_SECRET_KEY=<secret>
docker-run:
	docker run --rm \
	  -e ALPACA_API_KEY=$(ALPACA_API_KEY) \
	  -e ALPACA_SECRET_KEY=$(ALPACA_SECRET_KEY) \
	  -e ANTHROPIC_API_KEY=$(ANTHROPIC_API_KEY) \
	  $(DOCKER_IMAGE):$(DOCKER_TAG) watch -s $(or $(SYMBOL),AAPL)
