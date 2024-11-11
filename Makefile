.PHONY: swagger-clean
swagger-clean:
	rm -rf docs/swagger

.PHONY: swagger
swagger:
	swag init -g cmd/server/main.go --parseDependency --parseInternal --output docs/swagger

.PHONY: up
up:
	docker compose up --build

.PHONY: down
down:
	docker compose down

.PHONY: run-server
run-server:
	go run cmd/server/main.go

.PHONY: run
run: swagger run-server

.PHONY: test test-verbose test-coverage

# Run all tests
test:
	go test ./...

# Run tests with verbose output
test-verbose:
	go test -v ./...

# Run tests with coverage report
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

