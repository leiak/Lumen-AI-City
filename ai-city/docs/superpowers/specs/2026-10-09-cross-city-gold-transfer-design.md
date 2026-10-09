# Cross-city gold transfer design

**Status:** Approved design (2026-10-09)
**ADR:** [ADR-0011](../../adr/ADR-0011-cross-city-gold-transfer.md)
**Scope:** protocol contract + bridge-ledger contract for 3.0 v2 roadmap item #2

## Goals

1. Move gold between independently deployed cities without losing funds.
2. Keep each city's PostgreSQL database authoritative for its local wallets.
3. Make retries safe at both the A2A wire level and the local ledger level.
4. Preserve a complete audit trail for players and cross-city clearing.
5. Use the existing mTLS A2A gateway as the cross-city trust boundary.

## Non-goals

- No token or gem transfers in this slice.
- No currency conversion.
- No cross-city atomic transaction or 2PC.
- No public browser-facing A2A API.
- No automatic netting between bridge positions.

## Terms

| Term | Meaning |
|---|---|
| `global_id` | Source-generated UUID shared by both city legs. |
| `outbound` | Source-city leg that reserved funds from a local wallet. |
| `inbound` | Destination-city leg that credited a local wallet. |
| `reserved` | Source funds are held; destination may not yet have credited them. |
| `credited` | Destination has completed its local wallet credit. |
| `settled` | Source knows the destination credit completed. |
| `refunded` | Source returned reserved funds after expiry. |

## Protocol

### RPCs

```proto
enum CrossCityTransferStatus {
  CROSS_CITY_TRANSFER_STATUS_UNSPECIFIED = 0;
  CROSS_CITY_TRANSFER_STATUS_RESERVED = 1;
  CROSS_CITY_TRANSFER_STATUS_CREDITED = 2;
  CROSS_CITY_TRANSFER_STATUS_SETTLED = 3;
  CROSS_CITY_TRANSFER_STATUS_REFUNDED = 4;
  CROSS_CITY_TRANSFER_STATUS_FAILED = 5;
}

message TransferCrossCityRequest {
  string transfer_id = 1;
  string source_city_id = 2;
  string source_user_id = 3;
  string destination_city_id = 4;
  string destination_user_id = 5;
  string currency = 6;
  int64 amount = 7;
  string idempotency_key = 8;
  string trace_id = 9;
  int64 reserved_at_ms = 10;
  int64 expires_at_ms = 11;
}

message GetCrossCityTransferRequest {
  string transfer_id = 1;
  string source_city_id = 2;
  string destination_city_id = 3;
  string idempotency_key = 4;
  string trace_id = 5;
}

message TransferCrossCityResponse {
  string transfer_id = 1;
  CrossCityTransferStatus status = 2;
  string error_code = 3;
  string message = 4;
  int64 occurred_at_ms = 5;
  string trace_id = 6;
}
```

### Validation

Both RPC handlers validate:

- `transfer_id` and `idempotency_key` are non-empty.
- `currency == "gold"`.
- `amount > 0 && amount <= 100000`.
- `source_city_id != destination_city_id`.
- `source_user_id != destination_user_id || source_city_id != destination_city_id`.
- `expires_at_ms > reserved_at_ms`.

The destination handler additionally rejects an expired request with
`CROSS_CITY_EXPIRED`. The source never treats an expired rejection as a credit.

### Transport and identity

Cross-city transfer RPCs use the existing mTLS channel. The gateway maps the
authenticated peer certificate identity to a city ID. A request is rejected if
its declared `source_city_id` does not match that mapped identity. The source
city ID must be configured locally; it is never trusted from a browser payload.

## State machine

```text
Source outbound:
  reserved -> settled
  reserved -> refunded
  reserved -> failed (only after a terminal validation/admin reconciliation decision)

Destination inbound:
  (none) -> credited
  credited -> credited (idempotent replay)
```

There is no destination-side refund in this slice. If a destination credit fails
validation, no ledger state changes and the source reservation eventually
expires and refunds.

## PostgreSQL contract

The migration lives in `packages/proto/pg-schema-3.0-cross-city-gold.sql`.

```sql
CREATE TABLE IF NOT EXISTS cross_city_transfer (
    id                  BIGSERIAL PRIMARY KEY,
    global_id           UUID NOT NULL,
    direction           TEXT NOT NULL CHECK (direction IN ('outbound','inbound')),
    source_city_id      TEXT NOT NULL,
    destination_city_id TEXT NOT NULL,
    source_user_id      TEXT NOT NULL,
    destination_user_id TEXT NOT NULL,
    currency            TEXT NOT NULL CHECK (currency = 'gold'),
    amount              BIGINT NOT NULL CHECK (amount > 0 AND amount <= 100000),
    status              TEXT NOT NULL CHECK (
                          (direction = 'outbound' AND status IN ('reserved','settled','refunded','failed'))
                          OR
                          (direction = 'inbound' AND status IN ('credited','failed'))
                        ),
    idempotency_key     TEXT NOT NULL,
    trace_id            TEXT,
    expires_at          TIMESTAMPTZ NOT NULL,
    reserved_at         TIMESTAMPTZ,
    credited_at         TIMESTAMPTZ,
    settled_at          TIMESTAMPTZ,
    refunded_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (direction, idempotency_key),
    UNIQUE (global_id, direction)
);

CREATE INDEX IF NOT EXISTS idx_cct_outbound_reconcile
    ON cross_city_transfer(expires_at)
    WHERE direction = 'outbound' AND status = 'reserved';
CREATE INDEX IF NOT EXISTS idx_cct_source_user
    ON cross_city_transfer(source_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cct_destination_user
    ON cross_city_transfer(destination_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS bridge_position (
    id            BIGSERIAL PRIMARY KEY,
    peer_city_id  TEXT NOT NULL,
    direction     TEXT NOT NULL CHECK (direction IN ('outbound','inbound')),
    currency      TEXT NOT NULL CHECK (currency = 'gold'),
    balance       BIGINT NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (peer_city_id, direction, currency)
);

CREATE TABLE IF NOT EXISTS bridge_ledger_entry (
    id                 BIGSERIAL PRIMARY KEY,
    transfer_global_id UUID NOT NULL,
    direction          TEXT NOT NULL CHECK (direction IN ('outbound','inbound')),
    peer_city_id       TEXT NOT NULL,
    currency           TEXT NOT NULL CHECK (currency = 'gold'),
    amount             BIGINT NOT NULL CHECK (amount != 0),
    balance_after      BIGINT NOT NULL,
    reason             TEXT NOT NULL CHECK (
                         reason IN ('reserve','credit','refund','reconciliation')
                       ),
    trace_id           TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bridge_entry_transfer
    ON bridge_ledger_entry(transfer_global_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bridge_entry_peer
    ON bridge_ledger_entry(peer_city_id, direction, created_at DESC);

ALTER TABLE transaction DROP CONSTRAINT IF EXISTS transaction_tx_type_check;
ALTER TABLE transaction ADD CONSTRAINT transaction_tx_type_check
    CHECK (tx_type IN (
      'player_transfer','npc_purchase','central_bank_emit','npc_sink',
      'cross_city_out','cross_city_in'
    ));
```

`bridge_position.balance` is signed:

| Leg | Successful movement | Meaning |
|---|---:|---|
| Source `outbound` | `+amount` | Source has gold reserved for a remote credit. |
| Destination `inbound` | `-amount` | Destination owes this clearing position to the federation. |

After both legs succeed, the two city-local balances sum to zero at federation
scope. A later netting system can settle these positions without changing player
wallet history.

## Local transaction boundaries

### Source reserve

One PostgreSQL transaction:

1. Lock sender wallet with `FOR UPDATE`.
2. Ensure `gold_balance >= amount`.
3. Insert `cross_city_transfer` with `direction='outbound'`, `status='reserved'`.
4. Debit sender wallet.
5. Upsert and increment `bridge_position` for `outbound`.
6. Insert `bridge_ledger_entry` with `reason='reserve'`.
7. Insert one player `transaction` row with `tx_type='cross_city_out'`.

If any step fails, the whole reservation rolls back.

### Destination credit

One PostgreSQL transaction:

1. Insert `cross_city_transfer` with `direction='inbound'`, `status='credited'`.
   A unique violation returns the existing credited result.
2. Create-or-lock destination wallet.
3. Credit destination wallet.
4. Upsert and decrement `bridge_position` for `inbound`.
5. Insert `bridge_ledger_entry` with `reason='credit'`.
6. Insert one player `transaction` row with `tx_type='cross_city_in'`.

If any later step fails, the credit rolls back and the source reservation remains
reconcilable.

### Source settle

One PostgreSQL transaction:

1. Lock the outbound transfer.
2. Require status `reserved`.
3. Update status to `settled` and set `settled_at`.

No wallet or bridge position changes occur at this point.

### Source refund

One PostgreSQL transaction:

1. Lock an expired outbound transfer in status `reserved`.
2. Update status to `refunded` and set `refunded_at`.
3. Credit sender wallet.
4. Decrement `bridge_position` for `outbound`.
5. Insert `bridge_ledger_entry` with `reason='refund'`.
6. Insert one positive player `transaction` row with `tx_type='cross_city_out'`.

## REST and internal API

The first implementation exposes these economy-service APIs:

| Method | Path | Caller | Purpose |
|---|---|---|---|
| POST | `/api/v1/cross-city-transfers` | signed-in source user | Reserve funds and publish the outbound leg. |
| GET | `/api/v1/cross-city-transfers/{global_id}` | source user, admin | Read local leg status. |
| POST | `/api/v1/cross-city-transfers/{global_id}/refund` | admin or reconciler | Refund an expired source reservation. |
| POST | `/internal/v1/cross-city-transfers/credit` | local A2A gateway | Credit the destination leg. |
| POST | `/internal/v1/cross-city-transfers/settle` | local A2A gateway | Mark a source leg settled after remote credit. |
| GET | `/internal/v1/cross-city-transfers/{global_id}` | local A2A gateway | Read local leg status for reconciliation. |

Internal endpoints use a dedicated service token, not a browser JWT.

## Error codes

| Code | Meaning |
|---|---|
| `CROSS_CITY_VALIDATION_FAILED` | Contract fields failed validation. |
| `CROSS_CITY_UNSUPPORTED_CURRENCY` | Currency is not enabled. |
| `CROSS_CITY_AMOUNT_OUT_OF_RANGE` | Amount is zero, negative, or above `100000`. |
| `CROSS_CITY_SAME_CITY` | Source and destination city are the same. |
| `CROSS_CITY_EXPIRED` | Destination received the request after expiry. |
| `CROSS_CITY_IDEMPOTENCY_CONFLICT` | Same key with a different transfer payload. |
| `CROSS_CITY_SOURCE_NOT_FOUND` | Source outbound leg does not exist. |
| `CROSS_CITY_INVALID_STATE` | Requested transition is not allowed. |

## Observability

Kafka events:

- `econ.crosscity.gold.reserved`
- `econ.crosscity.gold.credited`
- `econ.crosscity.gold.settled`
- `econ.crosscity.gold.refunded`

Each payload includes `global_id`, source/destination city and user IDs,
amount, trace ID, and the local leg status. Logs must include `global_id`,
`trace_id`, and local leg direction. A later observability slice can add
Prometheus counters and bridge-position gauges.

## Reconciliation

The source gateway runs a periodic reconciler:

1. Select outbound rows in `reserved` whose `expires_at` is in the past.
2. Call destination `GetCrossCityTransfer`.
3. If destination says `credited`, call local settle.
4. If destination says not found, call local refund.
5. If the destination is unreachable, leave the row `reserved` and retry later.

The destination never credits an expired request. Therefore the source may safely
refund after expiry once the destination confirms that no matching inbound leg
exists.

## Testing strategy

- **Proto tests:** field and enum presence, generated Go code compiles.
- **SQL tests:** constraints, unique indexes, and transaction-type extension.
- **Service tests:** reserve, credit, settle, refund, insufficient balance,
  duplicate replay, and idempotency conflict.
- **Gateway tests:** validation, mTLS city identity, expiry, and status mapping.
- **Reconciler tests:** missing transfer, credited transfer, unreachable peer,
  and refund after expiry.
- **Integration test:** source user debit, destination user credit, opposite
  bridge positions, and no duplicate wallet movement after retries.
