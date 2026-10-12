.PHONY: build run test test-integration lint vuln sqlc migrate-up dev-keys dev-token

build:
	go build -o bin/api ./cmd/api

run:
	go run ./cmd/api

test:
	go test -race ./...

test-integration:
	go test -race -tags=integration ./...

lint:
	test -z "$$(gofmt -l . | grep -v '^internal/db/sqlc/')"
	go vet ./...
	golangci-lint run

vuln:
	govulncheck ./...

sqlc:
	sqlc generate

migrate-up:
	migrate -path migrations -database "$$DATABASE_URL" up

dev-keys:
	openssl ecparam -name prime256v1 -genkey -noout -out dev/jwt.key
	openssl ec -in dev/jwt.key -pubout -out dev/jwt.pub

dev-token:
	@go run ./cmd/devtoken
