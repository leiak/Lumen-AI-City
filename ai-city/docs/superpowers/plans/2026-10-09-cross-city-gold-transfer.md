# Cross-city gold transfer implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the approved mTLS A2A transfer contract and the two-leg bridge-ledger Saga for gold.

**Architecture:** Economy-service owns local ledger transactions. A2A-gateway owns cross-city RPC, peer-city identity, and reconciliation. The source reserves, the destination credits idempotently, and the source settles or refunds. No distributed transaction is used.

**Tech Stack:** Go 1.23, protobuf/gRPC, Python 3.12 + FastAPI + asyncpg, PostgreSQL 16, Redis idempotency, Kafka events, mTLS.

**Spec:** `docs/superpowers/specs/2026-10-09-cross-city-gold-transfer-design.md`

## Global Constraints

- Currency must be `gold` only.
- `amount` must be greater than zero and no greater than `100000`.
- Cross-city RPC must require mTLS and map the peer certificate to a city ID.
- Every money mutation must occur in one local PostgreSQL transaction.
- Every retry must be idempotent by `(direction, idempotency_key)`.
- The destination must reject expired requests.
- The source may refund only after expiry and a destination not-found response.

---

### Task 1: Protocol contract

**Files:**

- Modify: `packages/proto/a2a.proto`
- Test: `apps/a2a-gateway/internal/a2asrv/cross_city_proto_test.go`

**Interfaces:**

- Produces: `a2av1.TransferCrossCityRequest`, `a2av1.GetCrossCityTransferRequest`, `a2av1.TransferCrossCityResponse`, `a2av1.CrossCityTransferStatus`.

- [x] **Step 1: Add proto messages**

Append the contract from the spec to `a2a.proto` and add these service methods:

```proto
rpc TransferCrossCity(TransferCrossCityRequest) returns (TransferCrossCityResponse);
rpc GetCrossCityTransfer(GetCrossCityTransferRequest) returns (TransferCrossCityResponse);
```

- [x] **Step 2: Regenerate Go code**

```bash
cd packages/proto
protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative a2a.proto
```

- [x] **Step 3: Run Go tests**

```bash
cd apps/a2a-gateway
go test ./...
```

- [x] **Step 4: Commit**

```bash
git add packages/proto/a2a.proto packages/proto/gen/go/a2a/v1
git commit -m "feat(a2a): add cross-city gold transfer contract"
```

### Task 2: Bridge-ledger migration

**Files:**

- Create: `packages/proto/pg-schema-3.0-cross-city-gold.sql`
- Test: `apps/economy-service/tests/test_cross_city_schema.py`

**Interfaces:**

- Produces: `cross_city_transfer`, `bridge_position`, `bridge_ledger_entry`; extends `transaction.tx_type`.

- [x] **Step 1: Write the schema test**

```python
def test_cross_city_schema_contains_bridge_tables():
    sql = Path("packages/proto/pg-schema-3.0-cross-city-gold.sql").read_text()
    for token in (
        "CREATE TABLE IF NOT EXISTS cross_city_transfer",
        "CREATE TABLE IF NOT EXISTS bridge_position",
        "CREATE TABLE IF NOT EXISTS bridge_ledger_entry",
        "cross_city_out",
        "cross_city_in",
    ):
        assert token in sql
```

- [x] **Step 2: Add the migration**

Use the full SQL contract in the spec, including all constraints and indexes.

- [x] **Step 3: Apply against PostgreSQL**

```bash
docker compose exec -T postgres psql -U aicity -d aicity \
  < packages/proto/pg-schema-3.0-cross-city-gold.sql
```

- [x] **Step 4: Run tests**

```bash
cd apps/economy-service
PYTHONPATH=src python -m pytest tests/test_cross_city_schema.py -q
```

- [x] **Step 5: Commit**

```bash
git add packages/proto/pg-schema-3.0-cross-city-gold.sql apps/economy-service/tests/test_cross_city_schema.py
git commit -m "feat(economy): add cross-city bridge ledger schema"
```

### Task 3: Source reserve and refund

**Files:**

- Create: `apps/economy-service/src/economy_service/services/cross_city_service.py`
- Modify: `apps/economy-service/src/economy_service/schemas/economy.py`
- Modify: `apps/economy-service/src/economy_service/api/v1/cross_city.py`
- Modify: `apps/economy-service/src/economy_service/app.py`
- Test: `apps/economy-service/tests/test_cross_city_service.py`
- Test: `apps/economy-service/tests/test_api_cross_city.py`

**Interfaces:**

- Consumes: `WalletService`, `asyncpg.Pool`, Kafka producer.
- Produces: `CrossCityService.reserve()`, `CrossCityService.refund()`, `CrossCityService.get()`, schemas `CrossCityTransferRequest` and `CrossCityTransferResponse`.

- [x] **Step 1: Write failing service tests**

Cover:

```text
reserve debits sender, writes outbound reserved, increments bridge position
reserve rejects insufficient balance and writes no rows
duplicate reserve returns the existing leg without a second debit
refund credits sender, decrements bridge position, and marks refunded
refund rejects a non-expired or already-settled leg
```

- [x] **Step 2: Implement the service**

Use one `async with pool.acquire() as conn, conn.transaction():` block per mutation. Lock the wallet or transfer row before changing balances. Re-read an existing row on unique violation and compare all contract fields before returning it.

- [x] **Step 3: Add REST schemas and routes**

Expose reserve, get, and refund routes from the spec. Reserve requires the signed-in source user. Refund allows admin or the reconciler service identity.

- [x] **Step 4: Run tests**

```bash
cd apps/economy-service
PYTHONPATH=src python -m pytest tests/test_cross_city_service.py tests/test_api_cross_city.py -q
```

- [ ] **Step 5: Commit**

```bash
git add apps/economy-service/src/economy_service apps/economy-service/tests
git commit -m "feat(economy): reserve and refund cross-city gold"
```

### Task 4: Destination credit and source settle

**Files:**

- Modify: `apps/economy-service/src/economy_service/services/cross_city_service.py`
- Modify: `apps/economy-service/src/economy_service/api/v1/cross_city.py`
- Test: `apps/economy-service/tests/test_cross_city_service.py`
- Test: `apps/economy-service/tests/test_api_cross_city.py`

**Interfaces:**

- Produces: `CrossCityService.credit_inbound()`, `CrossCityService.settle_outbound()`.

- [x] **Step 1: Add failing tests**

Cover:

```text
credit_inbound creates destination wallet and credits it
credit_inbound increments the destination transaction history
duplicate inbound replay returns credited without a second wallet credit
credit_inbound rejects expired requests
settle_outbound transitions reserved to settled without changing balances
```

- [x] **Step 2: Implement internal routes**

Expose credit, settle, and get on `/internal/v1/cross-city-transfers/*`. Require a service token. Do not accept a browser JWT for these routes.

- [x] **Step 3: Run economy tests**

```bash
cd apps/economy-service
PYTHONPATH=src python -m pytest tests/test_cross_city_service.py tests/test_api_cross_city.py -q
```

- [x] **Step 4: Commit**

```bash
git add apps/economy-service/src/economy_service apps/economy-service/tests
git commit -m "feat(economy): credit and settle cross-city gold legs"
```

### Task 5: Gateway RPC handlers

**Files:**

- Create: `apps/a2a-gateway/internal/crosscity/money.go`
- Create: `apps/a2a-gateway/internal/crosscity/money_test.go`
- Modify: `apps/a2a-gateway/internal/a2asrv/service.go`
- Modify: `apps/a2a-gateway/cmd/main.go`

**Interfaces:**

- Consumes: generated `a2av1` messages and `CrossCityService` HTTP endpoints.
- Produces: `TransferCrossCity`, `GetCrossCityTransfer`, and `MoneyClient`.

- [ ] **Step 1: Write failing gateway tests**

Cover:

```text
valid request calls local economy credit endpoint
expired request returns CROSS_CITY_EXPIRED without calling economy
peer city mismatch is rejected
duplicate request returns the original credited response
GetCrossCityTransfer is read-only
```

- [ ] **Step 2: Implement handlers and HTTP client**

Inject an `EconomyClient` into `a2asrv.Service`. Use context timeout, service token, and the configured local economy base URL. Map HTTP status and economy error codes to gRPC status codes while preserving `error_code` in the response.

- [ ] **Step 3: Configure the service**

Add environment variables:

```text
CITY_ID=city_a
ECONOMY_SERVICE_URL=http://economy-service:8005
CROSS_CITY_INTERNAL_TOKEN=dev-cross-city-token
CROSS_CITY_RESERVATION_TTL=10m
```

- [ ] **Step 4: Run tests**

```bash
cd apps/a2a-gateway
go test ./...
```

- [ ] **Step 5: Commit**

```bash
git add apps/a2a-gateway
git commit -m "feat(a2a): implement cross-city transfer RPC handlers"
```

### Task 6: Source orchestration and reconciler

**Files:**

- Modify: `apps/a2a-gateway/internal/crosscity/money.go`
- Create: `apps/a2a-gateway/internal/crosscity/reconciler.go`
- Create: `apps/a2a-gateway/internal/crosscity/reconciler_test.go`
- Modify: `apps/a2a-gateway/cmd/main.go`

**Interfaces:**

- Consumes: local economy reserve/settle/refund endpoints and remote A2A RPCs.
- Produces: `StartCrossCityReconciler(ctx, deps, interval)`.

- [ ] **Step 1: Add orchestration tests**

Cover:

```text
reserve then remote credit then local settle
remote timeout leaves reservation pending
remote credited response settles source
expired not-found reservation refunds source
unreachable peer leaves reservation pending
```

- [ ] **Step 2: Implement reconciler**

Poll every 30 seconds. Select expired outbound reservations from the local economy service. For each row, call remote `GetCrossCityTransfer`. Settle on `credited`, refund on not-found, and retry on transport failure.

- [ ] **Step 3: Run tests**

```bash
cd apps/a2a-gateway
go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add apps/a2a-gateway
git commit -m "feat(a2a): reconcile cross-city gold reservations"
```

### Task 7: Integration and documentation

**Files:**

- Create: `apps/economy-service/scripts/acceptance_cross_city_gold_v1.py`
- Modify: `ai-city/docs/3.0-ROADMAP.md`
- Modify: `ai-city/CHANGELOG-3.0.md`
- Modify: `ai-city/README.md` if the service feature list needs an update

**Interfaces:**

- Consumes: all prior tasks.
- Produces: repeatable acceptance script and GA-ready documentation.

- [ ] **Step 1: Add acceptance script**

The script must verify in order:

```text
1 reserve source
2 destination credit
3 source settle
4 source wallet decreased once
5 destination wallet increased once
6 bridge positions are opposite
7 duplicate inbound replay changes nothing
8 get status is read-only
```

- [ ] **Step 2: Run full regression**

```bash
cd apps/economy-service
PYTHONPATH=src python -m pytest tests -q

cd ../../apps/a2a-gateway
go test ./...
```

- [ ] **Step 3: Run acceptance**

```bash
docker compose exec -T economy-service \
  python scripts/acceptance_cross_city_gold_v1.py
```

- [ ] **Step 4: Update roadmap and changelog**

Mark roadmap item #2 as **design accepted / implementation in progress** only when all implementation tasks are complete; do not mark GA before the acceptance script passes.

- [ ] **Step 5: Commit**

```bash
git add apps/economy-service ai-city/docs ai-city/CHANGELOG-3.0.md ai-city/README.md
git commit -m "feat(economy): cross-city gold acceptance and docs"
```

## Definition of Done

- [ ] Proto contract and generated code are committed.
- [ ] Bridge-ledger migration is idempotent and applied.
- [ ] Economy reserve, credit, settle, and refund are transactional and tested.
- [ ] A2A transfer and status RPCs use mTLS peer-city identity.
- [ ] Source reconciler settles credited legs and refunds expired missing legs.
- [ ] Full economy and gateway test suites pass.
- [ ] Cross-city acceptance script reports all steps PASS.
- [ ] Roadmap, changelog, and ADR reflect implementation status.
