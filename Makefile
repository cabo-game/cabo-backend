.PHONY: run-roomsvc run-authsvc build test vet docker-up docker-down

# Fails fast with a clear message instead of letting a missing .env show up
# as a confusing "required env var not set" error from inside the binary.
check-env:
	@test -f .env || (echo ".env not found — run: cp .env.example .env" && exit 1)

run-roomsvc: check-env
	set -a; . ./.env; set +a; go run ./cmd/roomsvc

run-authsvc: check-env
	set -a; . ./.env; set +a; go run ./cmd/authsvc

build:
	go build ./...

test:
	go test ./... -race

vet:
	go vet ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down
