include .env
export $(shell sed 's/=.*//' .env)

.DEFAULT_GOAL := build

test:
	go test -v --short --cover ./...
.PHONY: test

e2e:
	go test -v ./api/e2e
.PHONY: unit

fmt:
	go fmt ./...
.PHONY: fmt

lint: fmt
	golangci-lint run
.PHONY: lint

vet: fmt
	go vet ./...
 	$(shell shadow ./...)
.PHONY: vet

build: lint vet
	go build -o main .
