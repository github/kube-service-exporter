GO=go
FILES=`glide novendor`

.PHONY: all vet test build clean coverage

all: vet test build

vet:
	$(GO) vet $(FILES)

build:
	$(GO) build -o bin/kube-service-exporter

test: vet
	$(GO) test -race -cover -v $(FILES)

clean:
	rm -vrf bin
	rm coverage.out

coverage: test
	go tool cover -html=coverage.out
