GO=go
GIT_COMMIT := $(shell git rev-parse HEAD)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
BUILD_TIME := $(shell date --iso-8601=seconds --utc)
FILES := $(shell go list ./.../)

.PHONY: all vet test build clean coverage

all: vet test build

vet:
	$(GO) vet $(FILES)

build:
	$(GO) build \
		-o bin/kube-service-exporter \
		-ldflags "-X main.GitCommit=$(GIT_COMMIT) -X main.GitBranch=$(GIT_BRANCH) -X main.BuildTime=$(BUILD_TIME)"

test: vet
	$(GO) test -race -cover -v $(FILES)

clean:
	rm -vrf bin
	rm coverage.out

coverage:
	go tool cover -html=coverage.out
