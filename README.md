# Booking Platform - Go Microservices Demo

A high-throughput booking platform built in **Go** with a **microservices architecture**, designed to demonstrate distributed systems patterns used in real-world e-commerce/travel platforms.

## Architecture

The platform is split into core services to enforce separation of concerns and allow independent scaling. Each service follows the **database-per-service** pattern, avoiding tight coupling and enabling fault isolation — at the cost of requiring eventual consistency.

```
                     ┌─────────────────┐
                     │   API Gateway   │  (JWT auth, routing)
                     └────────┬────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
   ┌────▼─────┐        ┌──────▼──────┐      ┌──────▼──────┐
   │  Search  │        │   Pricing   │      │ Reservation │
   │ Service  │        │   Service   │      │   Service   │
   └────┬─────┘        └──────┬──────┘      └──────┬──────┘
        │                     │                     │
   ┌────▼─────┐        ┌──────▼──────┐      ┌──────▼──────┐
   │ MongoDB  │        │ PostgreSQL  │      │ PostgreSQL  │
   └──────────┘        └─────────────┘      └─────────────┘
                                                    │
                                              ┌─────▼──────┐
                                              │  RabbitMQ  │  (event bus)
                                              └─────┬──────┘
                                                    │
                                          ┌─────────▼─────────┐
                                          │  Notification Svc │
                                          └───────────────────┘
```

## Services

| Service | Responsibility | DB |
|---|---|---|
| **API Gateway** | Single entry point, JWT auth, request routing | — |
| **Search** | Property/flight search with caching | MongoDB |
| **Pricing** | Real-time pricing calculations | PostgreSQL + Redis |
| **Reservation** | Booking lifecycle, payment orchestration | PostgreSQL |
| **Notification** | Async event consumer for emails/SMS | — |

## Communication Patterns

- **Synchronous REST** — for immediate consistency (e.g., fetching real-time pricing during reservation)
- **Asynchronous events (RabbitMQ)** — for workflows that don't need immediate ack (e.g., `ReservationCreated → Notification`, `AuditLog`)
- **JWT propagation** — identity passed via headers; downstream services stateless

## Resilience Patterns

- Circuit breakers for upstream supplier API calls
- Exponential backoff with jitter on retries
- Graceful degradation (cached fallback if pricing service is slow)

## Observability

- **Prometheus** — metrics scraping
- **Grafana** — dashboards
- **Structured JSON logging** — log aggregation ready
- **Distributed tracing** — OpenTelemetry instrumented (request flow across services)

## Deployment

- Fully **containerized** with Docker
- Deployed on **Kubernetes** via Helm-style manifests
- CI/CD ready (GitHub Actions workflow included)
- Tested locally with `docker-compose`

## Getting Started

```bash
# Run locally with docker-compose
docker-compose up

# Or deploy to Kubernetes
kubectl apply -f deploy/k8s/

# Run tests
make test
```

## Tech Stack

- **Language:** Go 1.21+
- **Framework:** Gin (HTTP), gRPC (inter-service)
- **Databases:** PostgreSQL, MongoDB, Redis
- **Messaging:** RabbitMQ (or AWS SQS/SNS in production)
- **Infra:** Docker, Kubernetes, Helm
- **Observability:** Prometheus, Grafana, OpenTelemetry

## Project Structure

```
booking-platform/
├── services/
│   ├── search/         # Search microservice
│   ├── pricing/        # Pricing microservice
│   ├── reservation/    # Reservation microservice
│   └── notification/   # Notification consumer
├── gateway/            # API Gateway
├── deploy/
│   ├── docker/         # Dockerfiles + docker-compose
│   └── k8s/            # Kubernetes manifests
├── scripts/            # Helper scripts
└── docs/               # Architecture & API docs
```

## Why This Project

This demo reflects patterns I've used in production at scale:
- 50,000+ daily queries with sub-200ms response times
- MongoDB pipeline tuning bringing latency from 280ms to 95ms
- Resilience across 15+ third-party supplier integrations

Built as a learning resource and reference architecture for Go microservices.

## License

MIT
