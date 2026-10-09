# ADR-0011: 3.0 v4 — Cross-city gold transfer

**Status:** Accepted (2026-10-09)
**Date:** 2026-10-09
**Deciders:** fang, AI city core team
**Depends on:**

- [ADR-0007](0007-2.0-scope-llm-federation-memory-saga.md) — cross-city A2A and mTLS federation.
- [ADR-0008](0008-3.0-economy-scope.md) — economy service, wallet, transfer, and ledger baseline.
- [ADR-0010](ADR-0010-creator-marketplace.md) — creator-market transaction and idempotency patterns.

**Impact:**

- `packages/proto/a2a.proto`
- `packages/proto/pg-schema-3.0-cross-city-gold.sql`
- `apps/economy-service/`
- `apps/a2a-gateway/`
- `apps/api-gateway/`

## Context

Economy-service owns atomic local wallet transfers, but a transfer remains inside
one PostgreSQL database. Cross-city federation already provides authenticated
A2A calls and mTLS, yet it has no money-transfer RPC. A naive request that debits
the source city and credits the destination city in one call cannot survive a
network failure between the two commits. The result would be lost or duplicated
gold.

The first slice of roadmap item **3.0 v2 #2 Cross-city gold** therefore needs a
protocol contract and a bridge-ledger design before service implementation.

## Decision

Use a **two-phase bridge-ledger Saga** instead of distributed 2PC or a direct
remote wallet mutation.

1. **Reserve in the source city.** Atomically debit the sender, write an
   `outbound` cross-city transfer with status `reserved`, and increase the source
   city's outbound bridge position.
2. **Forward the reservation over mTLS A2A.** The source gateway calls
   `TransferCrossCity` on the destination gateway with the reservation contract.
3. **Credit in the destination city.** Atomically create an `inbound` transfer
   with status `credited`, credit the destination wallet, and increase the
   destination city's inbound bridge position.
4. **Settle the source.** After receiving a successful destination response, the
   source marks its outbound transfer `settled`.
5. **Refund only after expiry.** If the destination has no matching transfer and
   the reservation has expired, the source atomically refunds the sender and
   marks the outbound transfer `refunded`.

The v1 slice supports `gold` only, with a maximum of `100000` per transfer. The
protocol carries `currency` so later currencies can be enabled without another
wire-format change, but services reject every value except `gold` in this slice.

### Protocol contract

Add two unary RPCs to `A2AGateway`:

```proto
rpc TransferCrossCity(TransferCrossCityRequest) returns (TransferCrossCityResponse);
rpc GetCrossCityTransfer(GetCrossCityTransferRequest) returns (TransferCrossCityResponse);
```

`TransferCrossCityRequest` contains `transfer_id`, `source_city_id`,
`source_user_id`, `destination_city_id`, `destination_user_id`, `currency`,
`amount`, `idempotency_key`, `trace_id`, `reserved_at_ms`, and
`expires_at_ms`. The response contains the same `transfer_id`, resolved
`status`, `error_code`, `message`, and `occurred_at_ms`.

`GetCrossCityTransfer` is used by the source reconciler after a timeout. The
destination never executes a mutation for this RPC.

### Ledger contract

Each city stores the following tables in its local PostgreSQL database:

- `cross_city_transfer` — one row per local leg; `UNIQUE(direction,
  idempotency_key)` and `UNIQUE(global_id, direction)` make retries safe.
- `bridge_position` — one signed clearing balance per peer city, direction, and
  currency.
- `bridge_ledger_entry` — append-only bridge-position history.

The player-facing `transaction.tx_type` check is extended with
`cross_city_out` and `cross_city_in`. A source reservation writes one negative
`cross_city_out` transaction; a destination credit writes one positive
`cross_city_in` transaction. Bridge positions are reconciled independently from
player transaction history.

### Failure rules

- The destination is idempotent on `(direction='inbound', idempotency_key)`.
- The destination rejects requests whose `expires_at_ms` is in the past.
- The source reconciler first calls `GetCrossCityTransfer`.
- The source refunds only when the reservation has expired and the destination
  has no matching transfer.
- If the destination reports `credited`, the source marks its outbound row
  `settled` without changing a wallet balance again.

## Alternatives

### Direct local transfer via a shared database

- **Pros:** one PostgreSQL transaction and no new protocol.
- **Cons:** breaks city autonomy and cannot represent independently deployed
  cities.

### Distributed two-phase commit

- **Pros:** appears atomic across databases.
- **Cons:** blocks participants during failures and turns A2A into a transaction
  coordinator.

### Burn-and-mint

- **Pros:** no bridge positions.
- **Cons:** makes each city's money supply depend on another city's availability
  and complicates central-bank audit.

## Consequences

### Positive

- No funds are lost silently during a network split.
- Retries and reconciliation are explicit and auditable.
- Each city keeps local wallet semantics and local PostgreSQL ownership.
- mTLS remains the transport security boundary.

### Negative

- A transfer is eventually consistent rather than instantly settled.
- Bridge positions require periodic reconciliation and monitoring.
- A duplicate or out-of-order destination response must be handled explicitly.

## Verification

- Protocol tests cover field validation, expiry, duplicate delivery, and status
  reporting.
- Economy tests cover reserve, credit, settle, refund, insufficient balance, and
  idempotent replay.
- Reconciler tests cover missing transfer, credited transfer, and refund after
  expiry.
- A two-city integration test proves one transfer produces one debit, one
  credit, matching bridge positions, and no second player debit or credit.
