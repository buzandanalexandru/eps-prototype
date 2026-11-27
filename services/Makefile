# Go data plane (S2). Run from services/.
GO ?= go

.PHONY: gen build vet test test-short image migrate

## gen: regenerate the server stubs from api/openapi.yaml
gen:
	cd internal/api && $(GO) tool oapi-codegen -config oapi-codegen.yaml ../../../api/openapi.yaml

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

## test: all Go tests (testcontainers: needs Docker for postgres:18 and redis:8)
test:
	$(GO) test -count=1 ./...

test-short:
	$(GO) test -short ./...

## image: build one image, e.g. make image BIN=core
image:
	docker build --target $(BIN) --build-arg COMMIT=$$(git rev-parse HEAD) -t eps-$(BIN):dev .

## migrate: apply both schemas to DATABASE_URL
migrate:
	$(GO) run ./cmd/migrate -control ../apps/web/db/migrations
