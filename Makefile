.PHONY: generate-clients

generate-clients: generate-swagger
	# Generate Go client
	openapi-generator generate -i api/openapi/swagger.json -g go -o pkg/client/go
	# Generate TypeScript client
	openapi-generator generate -i api/openapi/swagger.json -g typescript-axios -o pkg/client/typescript

.PHONY: swagger-clean
swagger-clean:
	rm -rf docs/swagger

.PHONY: swagger
swagger:
	swag init -g cmd/server/main.go --parseDependency --parseInternal --output docs/swagger
