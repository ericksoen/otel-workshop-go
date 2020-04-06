GOOS=$(shell go env GOOS)

.DEFAULT_GOAL := run 

.PHONY: build 
build:
	go build -mod=vendor -o ./bin/otel-demo-$(GOOS) main.go

.PHONY: run 
run:
	go run -mod=vendor main.go

.PHONY: install
install:
	go mod download
	go mod vendor
