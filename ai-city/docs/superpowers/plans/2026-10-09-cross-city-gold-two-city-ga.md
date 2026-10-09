# Cross-City Gold Two-City GA Plan

## Tasks

1. **Server mTLS and peer identity**: add TLS options to the gRPC server, require and verify client certificates, enforce the expected peer CN, and add unit tests for secure and insecure startup paths.
2. **Certificate bootstrap**: provide a repeatable script that creates the shared CA and two server/client-auth certificates, including the expected SANs and CNs.
3. **City B overlay**: add an isolated PostgreSQL, Redis, economy-service, and a2a-gateway; wire City A and City B to each other with correct endpoints and identities.
4. **Two-city acceptance**: add a real acceptance binary that exercises health, reserve, commit, retry idempotency, rollback, and reconcile invariants.
5. **GA drill and documentation**: build and run both stacks, execute the drill, record results, and update the roadmap and changelog.

## Execution Record

- Task 1 complete: the gRPC server loads mTLS credentials, requires and verifies client certificates, rejects unexpected peer CNs, and refuses plaintext when a peer endpoint is configured.
- Task 2 complete: `scripts/bootstrap-cross-city-mtls.sh` creates and verifies the shared CA and both dual-use gateway certificates.
- Task 3 complete: `docker-compose.cross-city.yml` provides isolated City B infrastructure and configures both city identities and peer endpoints.
- Task 4 complete: `acceptance_cross_city_gold` covers both local and remote idempotency, remote lookup, wallet balance, settlement, and expired refund.
- Task 5 complete: the two-city drill passed; `go test ./...` passed; cross-city economy tests passed 30/30.

## Verification

- `go test ./...` in `apps/a2a-gateway`.
- Economy cross-city targeted tests.
- The two-city acceptance binary must exit zero.
