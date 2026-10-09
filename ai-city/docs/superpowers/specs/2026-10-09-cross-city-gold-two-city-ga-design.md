# Cross-City Gold Two-City GA Design

## Goal

Run City A and City B as independent local stacks and prove cross-city gold acceptance is production-like: mutual TLS, peer identity enforcement, isolated databases, real acceptance checks, and no distributed 2PC.

## Scope

- City A uses the existing `docker-compose.yml`.
- City B uses `docker-compose.cross-city.yml` with its own PostgreSQL, Redis, economy-service, and a2a-gateway.
- Both a2a-gateway processes expose gRPC over mTLS.
- Both peers verify that the client certificate CN matches the expected peer city gateway.
- Cross-city gold remains saga-based with local PostgreSQL transactions and replay idempotency.
- The acceptance binary drives real HTTP/gRPC flows and fails on any invariant violation.

## Non-Goals

- No distributed 2PC or shared database.
- No new assets, currencies, or product types.
- No deployment automation beyond a local two-city GA drill.

## Security

Server-side TLS is enabled with `tls.RequireAndVerifyClientCert` and TLS 1.2 as the minimum version. The CA signs server and client-auth capable certificates. Environment variables provide:

- `TLS_CERT`, `TLS_KEY`, `CLIENT_CA` for the gRPC server.
- `GRPC_TLS_CERT`, `GRPC_TLS_KEY`, `GRPC_TLS_CA` for outbound gRPC.
- `A2A_EXPECTED_PEER_CN` to enforce the peer identity.

City A uses CN `a2a-gateway-city-a`; City B uses CN `a2a-gateway-city-b`. Each side rejects the other's certificate if the CN differs from its configured peer CN.

## Data Safety

Every wallet and ledger mutation stays inside one local PostgreSQL transaction. The destination replay key remains `(direction, idempotency_key)`. Amounts remain constrained to `amount > 0 && amount <= 100000`.
