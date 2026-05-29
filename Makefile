.PHONY: build build-server fmt vet clean run-server run-server-spoof

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

clean:
	rm -f limithit

run-server:
	cd testserver && go run . --rate 5 --burst 5

run-server-spoof:
	cd testserver && go run . --rate 5 --burst 5 --trust-xff-cidr 127.0.0.0/8

all: fmt vet build
