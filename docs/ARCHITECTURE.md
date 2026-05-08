# Architecture Overview

## Design Principles

1. **Separation of concerns** — each service owns one domain
2. **Database-per-service** — no shared databases; services communicate via APIs/events
3. **Stateless services** — identity passed via headers; horizontal scale-friendly
4. **Eventual consistency** — accepted as a tradeoff for fault isolation
5. **Observability-first** — every service will be emiting structured logs + metrics

## Service Boundaries

### Search Service
- Read-heavy, indexed via MongoDB.
- Cached in Redis for hot queries
- Stateless — scales horizontally
- No writes from user requests (data populated via ETL from supplier feeds)

### Pricing Service
- Real-time price calculation with supplier integration
- Circuit breaker on upstream supplier APIs
- Stateless cache layer in Redis
- Returns `ExpiresAt` so clients can re-fetch

### Reservation Service
- Source of truth for bookings
- Synchronously calls pricing for final price confirmation
- Publishes `ReservationCreated` event to bus on success
- Writes to PostgreSQL with idempotency keys

### Notification Service
- Pure consumer — no inbound API from users
- Idempotent event handling (dedup by event ID)
- Sends emails/SMS via 3rd-party providers (e.g., SES, Twilio)

## Communication

| Pattern | When | Example |
|---|---|---|
| Sync REST | Need immediate response | `Reservation -> Pricing` |
| Async event | Fire-and-forget side effects | `Reservation -> Notification` |
| gRPC | Inter-service, high-throughput | (future enhancement) |

## Resilience

- **Circuit breakers** on upstream supplier calls (pricing service)
- **Exponential backoff with jitter** on all retries
- **Graceful degradation** — return cached fallback when supplier is down
- **Timeouts** — every HTTP client has a 2s timeout
- **Idempotency keys** — for safe retries on writes

## Observability

- Every service exposes `/metrics` (Prometheus format)
- Structured JSON logs to stdout (aggregated by log shipper in production)
- Request IDs propagated via headers for distributed tracing is possible
- Recommended: OpenTelemetry SDK for full distributed traces

## Datas Storage

| Service | Store | Why |
|---|---|---|
| Search | MongoDB | Schema-flexible, fast aggregation |
| Pricing | PostgreSQL + Redis | Strict pricing tables; cache for hot lookups |
| Reservation | PostgreSQL | Strong consistency for transactional data |

## Scaling Strategy 

- Stateless services scale via Kubernetes HPA on CPU is running
- Search service can scale to 10+ replicas during peak
- Pricing service is rate-limited by supplier APIs
- Reservation service uses connection pooling to Postgres

## Future Enhancements

- Service mesh (Istio/Linkerd) for mTLS + canary deploys
- gRPC for inter-service to reduce serialization overhead
- Event sourcing for reservation history
- Read replicas for pricing/reservation databases
