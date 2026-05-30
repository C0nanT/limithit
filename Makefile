.PHONY: build build-server fmt vet test test-all ci clean run-server run-server-spoof

build:
	go build -o limithit .

build-server:
	cd testserver && go build ./...

fmt:
	gofmt -w .
	gofmt -w testserver/

vet:
	go vet ./...
	cd testserver && go vet ./...

test:
	go test -race -cover ./internal/...

test-all:
	go test -race -cover ./internal/...
	cd testserver && go test -race -cover ./...

ci: fmt vet test-all

clean:
	rm -f limithit

run-server:
	cd testserver && go run . --rate 5 --burst 5

run-server-spoof:
	cd testserver && go run . --rate 5 --burst 5 --trust-xff-cidr 127.0.0.0/8

all: fmt vet build
