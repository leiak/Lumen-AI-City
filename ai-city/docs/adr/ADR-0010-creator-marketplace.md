# ADR-0010: 3.0 v3 — creator marketplace

- **Status:** Accepted / v3 GA (2026-10-08)
- **Scope:** economy-service marketplace + admin-portal creator/market/inventory UI
- **Depends on:** ADR-0008 economy v1, ADR-0009 admin-portal wallet UI

## Context

Economy v1 treats purchases as NPC-to-player product sales. Operators need a
creator path where creators publish NPC and Saga templates, buyers purchase
them, revenue is credited immediately, and admins can remove unsafe content
from the live market without breaking owned instances.

The existing stack already has wallet transfers, PG transactions, an admin
portal proxy layer, and seeded identities. The marketplace therefore extends
the economy service instead of introducing a separate service.

## Decision

1. **Three roles in one identity table.** `player.role` is constrained to
   `player`, `creator`, and `admin`. Creator and admin permissions live in the
   service authorization dependency, not in client-side UI state.

2. **PG slot 08 owns marketplace data.** Four new tables are used:
   `npc_template`, `saga_template`, `template_purchase`, and
   `creator_revenue`. Template content is JSON/YAML-backed; purchases are
   append-only; revenue is a separate ledger for reporting and audit.

3. **Economy service owns the domain API.** The admin portal only proxies
   `/v1/marketplace/*`. Browser code never receives the economy service token.

4. **HS256 subjects bridge both auth surfaces.** Admin-portal sessions keep
   `username` and `role`; marketplace-capable tokens also carry `sub`, the
   player UUID expected by economy-service. Economy-service resolves the row
   again and derives the authoritative role from PG.

5. **Templates are validated before persistence.** NPC OCEAN values are
   Pydantic-constrained, BT skeletons have depth/node limits, and Saga YAML is
   parsed with a safe loader that rejects Python object tags.

6. **Purchase is one transaction.** The service checks live status, locks the
   template, checks idempotency, locks wallets, debits the buyer, credits the
   creator, writes purchase/revenue/transaction rows, and emits Kafka after
   commit. Platform cut is zero in v1.

7. **Take-down is soft and admin-only.** A template status changes to
   `taken_down`, so it disappears from live browsing and new purchases return
   `410 / R_029`. Previously purchased inventory remains usable.

8. **Saga dependencies use an explicit v1 contract.** `npc_deps` stores NPC
   template IDs as text. The marketplace exposes dependency data today; the
   saga runtime's platform NPC pool integration remains a follow-up and fails
   fast when a dependency is unavailable.

## Public API

| Method | Path | Roles |
|---|---|---|
| POST | `/v1/marketplace/npc-templates` | creator, admin |
| GET | `/v1/marketplace/npc-templates` | public |
| GET | `/v1/marketplace/npc-templates/{id}` | signed-in |
| POST | `/v1/marketplace/npc-templates/{id}/take-down` | admin |
| POST | `/v1/marketplace/saga-templates` | creator, admin |
| GET | `/v1/marketplace/saga-templates` | public |
| GET | `/v1/marketplace/saga-templates/{id}` | public |
| POST | `/v1/marketplace/saga-templates/{id}/take-down` | admin |
| POST | `/v1/marketplace/purchase` | signed-in |
| GET | `/v1/marketplace/inventory/{user_id}` | owner, admin |
| GET | `/v1/marketplace/revenue/{creator_id}` | owner, admin |

## Admin portal surface

- `/creator/npc-templates` — creator list and creation form
- `/creator/saga-templates` — Saga list, dependency input, and creation form
- `/market` — NPC/Saga browsing and search
- `/market/npc-templates/{id}` — template detail and purchase
- `/market/saga-templates/{id}` — Saga detail and purchase
- `/inventory` — purchased template history
- `/api/marketplace/*` — authenticated server-side proxies, including the two
  admin-only take-down routes

## Error mapping

| Code | Meaning | HTTP |
|---|---|---|
| `R_026` | admin required / identity mismatch | 403 |
| `R_027` | creator/admin required | 403 |
| `R_028` | template not found | 404 |
| `R_029` | template taken down | 410 |
| `R_030` | insufficient balance | 402 |
| `R_031` | invalid BT skeleton | 400 |
| `R_032` | invalid Saga YAML | 400 |
| `R_033` | self-purchase forbidden | 403 |

## Consequences

### Positive

- Creators have one consistent publish/browse/purchase flow.
- Gold movement and revenue are atomic with the purchase.
- Admin can hide unsafe content without rewriting purchase history.
- The admin portal remains the only browser-facing API boundary.
- Economy tests and acceptance cover service, proxy, and full-flow behavior.

### Trade-offs

- Revenue is a separate table rather than inferred from transactions; the
  small write is accepted for clearer reporting and future cuts.
- Template status is soft rather than deleted, preserving inventory history.
- Saga runtime integration is intentionally not expanded in v3; the market
  stores and exposes the dependency contract first.
- Admin-portal session tokens must include `sub` for real cross-service calls;
  legacy tokens continue to work only for portal-local pages.

## Verification

- `pnpm --filter admin-portal test` — 136 passed.
- `pnpm --filter admin-portal typecheck` — passed.
- `python -m pytest tests -q` — 107 relevant economy tests passed (excluding
  four legacy files requiring a local `aiokafka` installation).
- `python scripts/acceptance_creator_market_v1.py` — 10/10 passed against
  dockerized postgres, redis, Kafka, and economy-service.
