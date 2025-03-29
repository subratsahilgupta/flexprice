<p align="center">
  <img align="center" src="./assets/flexprice logo.svg" height="40%" width="40%"  alt="fleprice logo"/>
</p>
<h3 align="center">
<b>
⚡️ Usage metering and billing for AI companies ⚡️
</b>
</h3 >
<p align="center">
🌟 Built with developers in mind, so you don't have to create
billing and metering from scratch. 🌟
</p>

---

<h4 align="center">

[![LinkedIn](https://img.shields.io/badge/linkedin-%230077B5.svg?style=for-the-badge&logo=linkedin&logoColor=white)](https://www.linkedin.com/company/flexpriceio)

</h4>

[Flexprice](https://flexprice.io) is a **developer first** answer to building and launching any pricing model and collecting usage data in real-time.

We have one goal - our friends should never have to write a billing system from scratch again.

## The struggle is real since AI is here

<img src="./assets/struggle.png" alt="Record Replay Testing"/>

In-house Billing Systems are a nightmare for Engineers. With rising use of AI workloads internally and building new AI features, it has become necessary to build metering and billing systems to track usage and costs.

Frequency of changes in pricing models is also increasing. These changes make it hard to manage existing subscriptions and require a lot of manual effort in terms of migrations and testing.


## The Solution: Flexprice

Usage metering, subscription management and infra cost into one stack

<img src="./assets/complex-iterations.png" alt="Record Replay Testing"/>



Developers don't need to spend **months** building internal metering and rating systems. 

Flexprice offers APIs, SDKs, self serve UI widgest, internal dashboards and more to help you get started in minutes. With Flexprice, you can:

- **Evolve your pricing with flexibility**: We support Usage-based pricing, Minimum commitments, Volume discounts, Prepaid Credits, Upgrades and downgrades, etc.

- **Real-time overview AI Infrastructure Costs**: Optimize revenue operations and measure LLM Token Usage, GPU Time, Compute Hours, API Call Frequency and Duration, etc

- **Custom Invoice and Billing logic**: Raise invoices, manage billing cycles, apply discounts, taxes, grace period, payment terms, etc


<img src="./assets/open-arch.png" alt="Flexprice Dashboard"/>

### 🚀 Setting up Flexprice from source for development and contributions

To run Flexprice for local development or running from source, you will need

1. [Golang](https://go.dev/)
2. [Docker](https://www.docker.com/) and [Docker Compose](https://docs.docker.com/compose/)
3. Any of the below supported platform environments:
    1. [Linux based environment](https://en.wikipedia.org/wiki/Comparison_of_Linux_distributions)
    2. [OSX (Darwin) based environment](https://en.wikipedia.org/wiki/MacOS)
    3. WSL under Windows

#### Quick Setup with Docker Compose

The easiest way to get started is using our Docker Compose setup:

```bash
# Clone the repository
git clone https://github.com/flexprice/flexprice
cd flexprice

# Set up the complete development environment
make dev-setup
```

This will:
1. Start all required infrastructure (PostgreSQL, Kafka, ClickHouse, Temporal)
2. Build the FlexPrice application image
3. Run database migrations and initialize Kafka
4. Start all FlexPrice services (API, Consumer, Worker)

Once complete, you can access:
- FlexPrice API: http://localhost:8080
- Temporal UI: http://localhost:8088
- Kafka UI: http://localhost:8084 (with profile 'dev')
- ClickHouse UI: http://localhost:8123

#### Useful Commands

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

#### Running Without Docker

If you prefer to run the application directly:

```bash
# Start the required infrastructure
docker compose up -d postgres kafka clickhouse temporal temporal-ui

# Run the application locally
go run cmd/server/main.go
```

### Code Architecture

See [CONTRIBUTING.md](CONTRIBUTING.md) for more details on the code architecture and design principles.


## 👨🏻‍💻 Let's Build Together! 👩🏻‍💻

Whether you're a newbie coder or a wizard 🧙‍♀️, your perspective is golden. Take a peek at our:

📜 [Contribution Guidelines](CONTRIBUTING.md)

🏗️ [Local Development Setup](SETUP.md)

❤️ [Code of Conduct](CODE_OF_CONDUCT.md)


## ✨ Resources!

📖 [API Docs](https://docs.flexprice.io/)

## 🛠️ API SDKs

Flexprice provides SDKs for multiple languages to make integration easier. These SDKs are automatically generated from our OpenAPI specifications.

### Generating SDKs

To generate the SDKs, run:

```bash
make generate-sdks
```

This will create SDKs for:
- JavaScript (npm)
- Python (PyPI)
- Go (GitHub)

### Versioning SDKs

To update the version of all SDKs:

```bash
make sdk-version VERSION=1.0.0
```

### Publishing SDKs

To publish all SDKs with a new version:

```bash
make sdk-publish-all-with-version VERSION=1.0.0
```

Or publish individual SDKs:

```bash
make sdk-publish-js  # Publish JavaScript SDK to npm
make sdk-publish-py  # Publish Python SDK to PyPI
make sdk-publish-go  # Prepare Go SDK for publishing
```

For more details on SDK generation and publishing, see the [API SDK documentation](api/README.md).
