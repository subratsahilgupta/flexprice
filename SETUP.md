# FlexPrice Setup Guide

## Prerequisites

- [Golang](https://go.dev/)
- [Docker](https://www.docker.com/) and [Docker Compose](https://docs.docker.com/compose/)
- Supported platforms:
  - Linux-based environment
  - macOS (Darwin)
  - WSL under Windows

## Quick Setup with Docker Compose

The easiest way to get started is using our Docker Compose setup:

```bash
# Clone the repository
git clone https://github.com/flexprice/flexprice
cd flexprice

# Set up the complete development environment
make dev-setup
```

This command will:
1. Start all required infrastructure (PostgreSQL, Kafka, ClickHouse, Temporal)
2. Build the FlexPrice application image
3. Run database migrations and initialize Kafka
4. Start all FlexPrice services (API, Consumer, Worker)

## Accessing Services

Once setup is complete, you can access:
- FlexPrice API: http://localhost:8080
- Temporal UI: http://localhost:8088
- Kafka UI: http://localhost:8084 (with profile 'dev')
- ClickHouse UI: http://localhost:8123

## Useful Commands

```bash
# Restart only the FlexPrice services
make restart-flexprice

# Stop all services
make down

# Clean everything and start fresh
make clean-start

# Build the FlexPrice image separately
make build-image

# Start only the FlexPrice services
make start-flexprice
```

## Running Without Docker

If you prefer to run the application directly:

```bash
# Start the required infrastructure
docker compose up -d postgres kafka clickhouse temporal temporal-ui

# Run the application locally
go run cmd/server/main.go
```

## Development Credentials

### PostgreSQL
- Host: localhost
- Port: 5432
- Database: flexprice
- Username: flexprice
- Password: flexprice123

### ClickHouse
- Host: localhost
- Port: 9000
- Database: flexprice
- Username: flexprice
- Password: flexprice123

### Kafka
- Bootstrap Server: localhost:29092
- UI: http://localhost:8084 (with profile 'dev')

## API Documentation

The API documentation is available in OpenAPI 3.0 format at `docs/swagger/swagger-3-0.json`.

### Setting up Postman Collection

1. Open Postman
2. Click on "Import" in the top left
3. Select "Import File"
4. Choose `docs/swagger/swagger-3-0.json`
5. Click "Import"
6. Create a new environment for local development:
   - Name: Local
   - Variable: `baseUrl`
   - Initial Value: `http://localhost:8080/v1`
   - Current Value: `http://localhost:8080/v1`

   - Variable: `apiKey`
   - Initial Value: `0cc505d7b917e0b1f25ccbea029dd43f4002edfea46b7f941f281911246768fe`
   - Current Value: `0cc505d7b917e0b1f25ccbea029dd43f4002edfea46b7f941f281911246768fe`

## Troubleshooting

### Docker Issues
1. Ensure Docker is running properly:
```bash
docker info
```

2. Check the status of all containers:
```bash
docker compose ps
```

3. View logs for a specific service:
```bash
docker compose logs [service_name]
```

### Database Connection Issues
1. Check database logs:
```bash
docker compose logs postgres
docker compose logs clickhouse
```

2. Verify the database is running:
```bash
docker compose ps postgres
docker compose ps clickhouse
```

### Kafka Issues
1. Verify Kafka is running:
```bash
docker compose logs kafka
```

2. Check topic list:
```bash
docker compose exec kafka kafka-topics --bootstrap-server kafka:9092 --list
```

3. View Kafka UI at http://localhost:8084

## Additional Resources

- [Contribution Guidelines](https://github.com/flexprice/flexprice/blob/main/CONTRIBUTING.md)
- [API Documentation](https://docs.flexprice.io/)
- [Code of Conduct](https://github.com/flexprice/flexprice/blob/main/CODE_OF_CONDUCT.md)
- [FlexPrice Website](https://flexprice.io)
