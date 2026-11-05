SHELL := /bin/sh

.PHONY: dev build test typecheck fmt fmt-check verify

dev:
	WAILS_DEV_ALLOWED_HOSTS=wails.localhost wails dev

build:
	wails build -clean

test:
	go test -tags webkit2_41 ./...
	go vet -tags webkit2_41 ./...

typecheck:
	npm --prefix frontend run typecheck
	npm --prefix frontend run build

fmt:
	gofmt -w main.go internal
	npm --prefix frontend run format

fmt-check:
	test -z "$$(gofmt -l main.go internal)"
	npm --prefix frontend run format:check

verify: fmt-check typecheck test
