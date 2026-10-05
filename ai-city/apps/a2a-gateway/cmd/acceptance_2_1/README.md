# acceptance_2_1 — 2.0 Stage 2 NPC Streaming/Emotion Acceptance Binary

Replaces `acceptance_1_0` (Sprint 12) with a stage 2 acceptance targeting the
new NPC streaming + emotion pipeline (ws-gateway + api-gateway + Redis pub/sub).

## Plan

`docs/superpowers/plans/2026-10-05-2.0-stage2-stream-emotion.md` (T15-T17).

## Steps

| Step | Task  | Status             | Description                                    |
| ---- | ----- | ------------------ | ---------------------------------------------- |
| 1    | T15   | implemented (this) | Login demo player + verify token issued        |
| 2    | T16   | TODO               | 5 NPCs stream + sentence_idx 0/1/2 validation  |
| 3    | T16   | TODO               | emotion ∈ 8 classes validation                 |
| 4    | T17   | TODO               | npc_say_stream_done arrival                    |
| 5    | T17   | TODO               | 5/5 PASS summary                               |

## Dependencies

- `github.com/redis/go-redis/v9` — Redis pub/sub client (`npc_say_stream` channel)
- `github.com/aicity/a2a-gateway/internal/streamcheck` — `Validator.Login` /
  `Validator.SubscribeBeat` (built in T14).

## NPC roster

The 5 default seeded NPCs (city_a instance):

```
npc_wang_boss_001        — 王老板 / snack shop owner
npc_grace_healer_001     — 林大夫 / pharmacist
npc_snack_owner_001      — 张小吃 / snack stall
npc_book_keeper_001       — 李掌柜 / book keeper
npc_dance_leader_001     — 周舞 / dance leader
```

## Endpoints

- api-gateway: `http://api-gateway:8080` (env override `API_GATEWAY_URL`)
- redis: `redis:6379` (env override `REDIS_URL` — TBD)
- pub/sub channel: `aicity:npc:say_stream` (constant `streamcheck.ChannelSayStream`)

## Exit codes

| Code | Meaning                          |
| ---- | -------------------------------- |
| 0    | All 5 steps PASS                 |
| 1    | Step 1 (login) failed            |
| 2    | Step 2 (streaming) failed        |
| 3    | Step 3 (emotion) failed          |
| 4    | Step 4 (stream_done) failed      |
| 5    | Step 5 (summary) failed          |

## Usage

```bash
# Build:
cd apps/a2a-gateway && go build -o /tmp/acceptance_2_1 ./apps/a2a-gateway/cmd/acceptance_2_1

# Run (from inside a2a-gateway container after T17 Dockerfile update):
docker compose exec -T a2a-gateway /app/acceptance_2_1
```

## Notes

- T17 will add the Dockerfile `COPY --from=builder /out/acceptance_2_1` line and
  the corresponding build command.
- T17 will also wire the binary as a smoke-test entrypoint in `docker-compose.yml`
  if/when needed.