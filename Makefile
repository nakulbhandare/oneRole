.PHONY: dev-up dev-down dev-key run migrate discover test build fmt

dev-up:
	docker compose up -d mongodb nats redis object-storage

dev-down:
	docker compose down

dev-key:
	mkdir -p .local
	openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out .local/jwt-signing.pem
	chmod 600 .local/jwt-signing.pem

run:
	set -a; [ ! -f .env ] || . ./.env; set +a; go run .

migrate:
	set -a; [ ! -f .env ] || . ./.env; set +a; go run . migrate

discover:
	set -a; [ ! -f .env ] || . ./.env; set +a; go run . discover

test:
	GOCACHE=$${GOCACHE:-/private/tmp/oneRole-gocache} go test ./...

build:
	go build ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f -not -path './.git/*')
