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
