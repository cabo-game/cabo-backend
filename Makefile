.PHONY: run-roomsvc run-authsvc build test vet docker-up docker-down go

# Fails fast with a clear message instead of letting a missing .env show up
# as a confusing "required env var not set" error from inside the binary.
check-env:
	@test -f .env || (echo ".env not found — run: cp .env.example .env" && exit 1)

run-roomsvc: check-env
	set -a; . ./.env; set +a; go run ./cmd/roomsvc

run-authsvc: check-env
	set -a; . ./.env; set +a; go run ./cmd/authsvc

# this ... 3 dots command is specific to go it represents wildcard to recursively go to all package and execute the cmd (e.g build test etc)
build:
	go build ./... 
# - race command is for the race detection if while executing 2 test functions in different go routines,  they both concurrently write to same resource
# to get that information and exit that time we use -race
test:
	go test ./... -race

# static analysis of suspicious code
vet:
	go vet ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

go:
	$(MAKE) run-roomsvc & $(MAKE) run-authsvc & wait

