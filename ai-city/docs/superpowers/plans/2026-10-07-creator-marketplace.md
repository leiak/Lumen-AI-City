# 3.0 v3 创作者市场 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让创作者发布 NPC 模板与 Saga 模板，玩家用 gold 购买，创作者即时收到收入，admin 可下架。从「经济系统」升级为「创作者经济」。

**Architecture:** 扩 economy-service + admin-portal（PG slot 08）；3-role 身份 (player/creator/admin)；即时分成（commission=0 字段预留）；Saga runtime 拉 platform_npc_pool。

**Tech Stack:** FastAPI + asyncpg + aiokafka (Python) + Next.js 15 + react-query + vitest + Playwright。

**Estimated:** 40 tasks / 56 commits / 8 周。

---

## W1 — PG schema + role 扩展

> **执行修正（2026-10-08）**：`player.id` 是 UUID，模板外键改为 UUID；`role` 已在 Phase C.4 存在，迁移补充 `player/creator/admin` 约束。schema/seed 挂载使用 slot 09/10（slot 07/08 已被 economy 占用）。服务鉴权改为校验 HS256 JWT `sub` 并查询 DB 角色。

### Task 1: player.role 列扩展 + seeds

**Files:**
- Create: `packages/proto/pg-schema-3.0-creator-market.sql`
- Create: `db/seed/seed-creator-market.sql`
- Modify: `docker-compose.yml` (slot 08 mount)
- Test: `apps/economy-service/tests/test_role_check.py`

- [ ] **Step 1: Write the failing test**

`apps/economy-service/tests/test_role_check.py`:
```python
def test_player_default_role():
    from economy_service.auth import get_player_role
    # existing seeded user 'demo' should have role='player'
    assert get_player_role('demo') == 'player'
```

- [ ] **Step 2: Verify fail**

```bash
cd apps/economy-service && PYTHONPATH=src pytest tests/test_role_check.py -v
```
Expected: FAIL with "No module named 'economy_service.auth'"

- [ ] **Step 3: Create migration SQL**

`packages/proto/pg-schema-3.0-creator-market.sql`:
```sql
-- slot 08: 创作者市场 + 3-role 身份
ALTER TABLE player ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'player'
  CHECK (role IN ('player', 'creator', 'admin'));

CREATE TABLE npc_template (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      TEXT NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    name            TEXT NOT NULL,
    avatar_url      TEXT,
    ocean_json      JSONB NOT NULL,
    bt_skeleton     TEXT,
    product_catalog JSONB,
    price_gold      BIGINT NOT NULL CHECK (price_gold >= 10),
    status          TEXT NOT NULL DEFAULT 'live' CHECK (status IN ('live', 'taken_down')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_npc_template_creator ON npc_template(creator_id);
CREATE INDEX idx_npc_template_status ON npc_template(status) WHERE status='live';

CREATE TABLE saga_template (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      TEXT NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    name            TEXT NOT NULL,
    icon_url        TEXT,
    description     TEXT,
    yaml_content    TEXT NOT NULL,
    npc_deps        TEXT[] NOT NULL DEFAULT '{}',
    semantic_version TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'live' CHECK (status IN ('live', 'taken_down')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_saga_template_creator ON saga_template(creator_id);
CREATE INDEX idx_saga_template_status ON saga_template(status) WHERE status='live';

CREATE TABLE template_purchase (
    id              BIGSERIAL PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES player(id) ON DELETE CASCADE,
    template_kind   TEXT NOT NULL CHECK (template_kind IN ('npc', 'saga')),
    template_id     BIGINT NOT NULL,
    price_paid_gold BIGINT NOT NULL,
    idempotency_key TEXT UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_purchase_user ON template_purchase(user_id);

CREATE TABLE creator_revenue (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      TEXT NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    purchase_id     BIGINT NOT NULL REFERENCES template_purchase(id),
    amount_gold     BIGINT NOT NULL,
    platform_cut_gold BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_creator_revenue_creator ON creator_revenue(creator_id, created_at DESC);
```

- [ ] **Step 4: Create seed SQL**

`db/seed/seed-creator-market.sql`:
```sql
-- 2 creator users + 1 admin (passwords match existing seed pattern: bcrypt hash)
UPDATE player SET role = 'creator' WHERE username = 'demo';
INSERT INTO player (username, role, ...) VALUES ('creator_demo', 'creator', ...);
INSERT INTO player (username, role, ...) VALUES ('admin_demo', 'admin', ...);
-- (real bcrypt hashes generated via existing seed pipeline)
```

- [ ] **Step 5: Mount slot 08 in docker-compose**

`docker-compose.yml` — economy-service mounts, add:
```yaml
- ./packages/proto/pg-schema-3.0-creator-market.sql:/docker-entrypoint-initdb.d/08-creator-market.sql:ro
- ./db/seed/seed-creator-market.sql:/docker-entrypoint-initdb.d/seed-creator-market.sql:ro
```

- [ ] **Step 6: Apply migration locally**

```bash
docker compose exec -T postgres psql -U aicity -d aicity -f /docker-entrypoint-initdb.d/08-creator-market.sql
docker compose exec -T postgres psql -U aicity -d aicity -f /docker-entrypoint-initdb.d/seed-creator-market.sql
```
Expected: ALTER + CREATE + INSERT success

- [ ] **Step 7: Implement auth helper + pass test**

`apps/economy-service/src/economy_service/auth.py`:
```python
from fastapi import Depends, HTTPException
from economy_service.db import get_conn

async def get_player_role(username: str) -> str:
    conn = await get_conn()
    row = await conn.fetchrow("SELECT role FROM player WHERE username = $1", username)
    if not row:
        raise HTTPException(status_code=404, detail={"code": "R_404", "msg": "user not found"})
    return row["role"]

async def require_role(roles: list[str], username: str):
    role = await get_player_role(username)
    if role not in roles:
        raise HTTPException(status_code=403, detail={"code": "R_027", "msg": "creator role required"})
    return role
```

- [ ] **Step 8: Verify test passes**

```bash
cd apps/economy-service && PYTHONPATH=src pytest tests/test_role_check.py -v
```
Expected: PASS

- [ ] **Step 9: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add packages/proto/pg-schema-3.0-creator-market.sql \
        db/seed/seed-creator-market.sql \
        docker-compose.yml \
        apps/economy-service/src/economy_service/auth.py \
        apps/economy-service/tests/test_role_check.py
git commit -m "feat(market): PG slot 08 schema + 3-role extension + auth helper"
```

---

## W2 — NPC template CRUD

### Task 2: NPC template Pydantic schemas

**Files:**
- Create: `apps/economy-service/src/economy_service/schemas/marketplace_npc.py`
- Test: `apps/economy-service/tests/test_marketplace_npc_schemas.py`

- [ ] **Step 1: Write the failing test**

```python
from economy_service.schemas.marketplace_npc import NpcTemplateCreate
from pydantic import ValidationError
import pytest

def test_npc_template_create_valid():
    t = NpcTemplateCreate(
        name="Chef Wang",
        ocean_json={"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3},
        price_gold=100,
    )
    assert t.name == "Chef Wang"

def test_npc_template_price_too_low():
    with pytest.raises(ValidationError):
        NpcTemplateCreate(
            name="X",
            ocean_json={"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
            price_gold=5,  # < min 10
        )
```

- [ ] **Step 2: Verify fail → Step 3: Implement**

```python
# apps/economy-service/src/economy_service/schemas/marketplace_npc.py
from pydantic import BaseModel, Field, field_validator
from typing import Optional, List

class OceanJson(BaseModel):
    O: float = Field(ge=0.0, le=1.0)
    C: float = Field(ge=0.0, le=1.0)
    E: float = Field(ge=0.0, le=1.0)
    A: float = Field(ge=0.0, le=1.0)
    N: float = Field(ge=0.0, le=1.0)

class ProductItem(BaseModel):
    name: str
    price_gold: int = Field(ge=0)
    stock: Optional[int] = None

class NpcTemplateCreate(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    avatar_url: Optional[str] = None
    ocean_json: OceanJson
    bt_skeleton: Optional[str] = None
    product_catalog: Optional[List[ProductItem]] = None
    price_gold: int = Field(ge=10)

class NpcTemplateResponse(NpcTemplateCreate):
    id: int
    creator_id: str
    status: str
    created_at: str
    updated_at: str
```

- [ ] **Step 4: Verify pass → Step 5: Commit**

```bash
git commit -m "feat(market): NPC template Pydantic schemas"
```

---

### Task 3: NPC template validator (depth ≤ 10 + nodes ≤ 50)

**Files:**
- Create: `apps/economy-service/src/economy_service/services/template_validator.py`
- Test: `apps/economy-service/tests/test_template_validator.py`

- [x] **Step 1: Write failing tests**

```python
from economy_service.services.template_validator import validate_bt_skeleton, BTInvalidError
import pytest

def test_valid_bt():
    bt = '{"type":"sequence","children":[{"type":"action","name":"square"}]}'
    validate_bt_skeleton(bt)  # no exception

def test_too_deep_bt():
    # depth 11 nested sequences
    bt = '{"type":"sequence","children":[' * 11 + ']' * 11 + '}'
    with pytest.raises(BTInvalidError):
        validate_bt_skeleton(bt)

def test_too_many_nodes_bt():
    # 51 nodes
    children = ",".join(['{"type":"action","name":"x"}'] * 51)
    bt = '{"type":"sequence","children":[' + children + ']}'
    with pytest.raises(BTInvalidError):
        validate_bt_skeleton(bt)
```

- [x] **Step 2: Implement**

```python
import json
from typing import Any

class BTInvalidError(Exception):
    pass

MAX_DEPTH = 10
MAX_NODES = 50

def validate_bt_skeleton(bt_json: str):
    try:
        root = json.loads(bt_json)
    except json.JSONDecodeError as e:
        raise BTInvalidError(f"BT not valid JSON: {e}")
    nodes = [0]
    _walk(root, depth=0, nodes=nodes)
    if nodes[0] > MAX_NODES:
        raise BTInvalidError(f"BT exceeds {MAX_NODES} nodes")

def _walk(node: Any, depth: int, nodes: list[int]):
    if depth > MAX_DEPTH:
        raise BTInvalidError(f"BT depth exceeds {MAX_DEPTH}")
    nodes[0] += 1
    if isinstance(node, dict) and "children" in node:
        for child in node["children"]:
            _walk(child, depth + 1, nodes)
```

- [x] **Step 3: Verify + Commit**

```bash
git commit -m "feat(market): BT skeleton validator (depth ≤ 10 + nodes ≤ 50)"
```

---

### Task 4: NPC template service (CRUD)

**Files:**
- Create: `apps/economy-service/src/economy_service/services/marketplace_service.py`
- Test: `apps/economy-service/tests/test_marketplace_service.py`

- [ ] **Step 1: Write failing test**

```python
@pytest.mark.asyncio
async def test_create_npc_template():
    from economy_service.services.marketplace_service import create_npc_template
    tid = await create_npc_template(
        creator_id="u_demo",
        name="Chef Wang",
        ocean_json={"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3},
        price_gold=100,
    )
    assert tid > 0

@pytest.mark.asyncio
async def test_list_npc_templates_live_only():
    from economy_service.services.marketplace_service import list_npc_templates
    items = await list_npc_templates(status="live")
    for item in items:
        assert item["status"] == "live"

@pytest.mark.asyncio
async def test_takedown_npc_template_by_admin():
    from economy_service.services.marketplace_service import takedown_npc_template
    # insert one first
    tid = await create_npc_template(creator_id="u_demo", name="X", ocean_json={...}, price_gold=100)
    await takedown_npc_template(tid, admin_id="u_admin")
    item = await get_npc_template(tid)
    assert item["status"] == "taken_down"
```

- [ ] **Step 2: Implement**

```python
# apps/economy-service/src/economy_service/services/marketplace_service.py
from economy_service.db import get_conn

async def create_npc_template(creator_id, name, ocean_json, price_gold,
                                avatar_url=None, bt_skeleton=None, product_catalog=None):
    conn = await get_conn()
    row = await conn.fetchrow(
        """
        INSERT INTO npc_template (creator_id, name, avatar_url, ocean_json, bt_skeleton, product_catalog, price_gold)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id
        """,
        creator_id, name, avatar_url, ocean_json, bt_skeleton, product_catalog, price_gold,
    )
    return row["id"]

async def list_npc_templates(status="live", limit=50, offset=0):
    conn = await get_conn()
    rows = await conn.fetch(
        "SELECT * FROM npc_template WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
        status, limit, offset,
    )
    return [dict(r) for r in rows]

async def get_npc_template(tid):
    conn = await get_conn()
    return dict(await conn.fetchrow("SELECT * FROM npc_template WHERE id = $1", tid))

async def takedown_npc_template(tid, admin_id):
    conn = await get_conn()
    await conn.execute(
        "UPDATE npc_template SET status = 'taken_down', updated_at = NOW() WHERE id = $1",
        tid,
    )
```

- [ ] **Step 3: Verify + Commit**

```bash
git commit -m "feat(market): NPC template service (CRUD + takedown)"
```

---

### Task 5: NPC template API endpoints

**Files:**
- Create: `apps/economy-service/src/economy_service/api/v1/marketplace/npc_templates.py`
- Modify: `apps/economy-service/src/economy_service/app.py`
- Test: `apps/economy-service/tests/test_api_marketplace_npc.py`

- [ ] **Step 1: Write failing test**

```python
@pytest.mark.asyncio
async def test_post_npc_template_requires_creator_role():
    from fastapi.testclient import TestClient
    from economy_service.app import app
    client = TestClient(app)
    r = client.post(
        "/v1/marketplace/npc-templates",
        json={"name": "X", "ocean_json": {"O":0.5,"C":0.5,"E":0.5,"A":0.5,"N":0.5}, "price_gold": 100},
        headers={"Authorization": "Bearer player.jwt.here"},
    )
    assert r.status_code == 403
    assert r.json()["detail"]["code"] == "R_027"
```

- [ ] **Step 2: Implement endpoint**

```python
# apps/economy-service/src/economy_service/api/v1/marketplace/npc_templates.py
from fastapi import APIRouter, Depends, HTTPException, Header
from economy_service.auth import get_player_role, require_role
from economy_service.schemas.marketplace_npc import NpcTemplateCreate, NpcTemplateResponse
from economy_service.services import marketplace_service as svc
from economy_service.services.template_validator import validate_bt_skeleton, BTInvalidError

router = APIRouter(prefix="/v1/marketplace/npc-templates", tags=["can"])

@router.post("", response_model=dict)
async def create_template(body: NpcTemplateCreate, authorization: str = Header(...)):
    username = authorization.replace("Bearer ", "")
    role = await require_role(["creator", "admin"], username)
    if body.bt_skeleton:
        try:
            validate_bt_skeleton(body.bt_skeleton)
        except BTInvalidError as e:
            raise HTTPException(status_code=400, detail={"code": "R_031", "msg": str(e)})
    tid = await svc.create_npc_template(
        creator_id=username,
        name=body.name,
        ocean_json=body.ocean_json.model_dump(),
        price_gold=body.price_gold,
        avatar_url=body.avatar_url,
        bt_skeleton=body.bt_skeleton,
        product_catalog=[p.model_dump() for p in body.product_catalog] if body.product_catalog else None,
    )
    return {"id": tid}

@router.get("", response_model=list)
async def list_templates(status: str = "live", limit: int = 50, offset: int = 0):
    return await svc.list_npc_templates(status=status, limit=limit, offset=offset)

@router.get("/{tid}", response_model=dict)
async def get_template(tid: int):
    item = await svc.get_npc_template(tid)
    if not item:
        raise HTTPException(status_code=404, detail={"code": "R_028", "msg": "template not found"})
    return item

@router.post("/{tid}/take-down")
async def take_down(tid: int, authorization: str = Header(...)):
    username = authorization.replace("Bearer ", "")
    await require_role(["admin"], username)
    await svc.takedown_npc_template(tid, admin_id=username)
    return {"status": "taken_down"}
```

- [ ] **Step 3: Register router in app.py**

```python
# Modify apps/economy-service/src/economy_service/app.py
from economy_service.api.v1.marketplace import npc_templates
app.include_router(npc_templates.router)
```

- [ ] **Step 4: Verify test + full pytest**

```bash
cd apps/economy-service && PYTHONPATH=src pytest tests/test_api_marketplace_npc.py -v
cd apps/economy-service && PYTHONPATH=src pytest tests/ -v
```
Expected: 8+ tests pass

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(market): NPC template API endpoints + role check"
```

---

## W3 — Saga template CRUD

### Task 6: Saga template schemas + YAML validator

**Files:**
- Create: `apps/economy-service/src/economy_service/schemas/marketplace_saga.py`
- Modify: `apps/economy-service/src/economy_service/services/template_validator.py`
- Test: `apps/economy-service/tests/test_marketplace_saga_schemas.py`

- [ ] **Step 1: Write failing tests**

```python
from economy_service.services.template_validator import validate_saga_yaml, YamlInvalidError
import pytest

def test_valid_yaml():
    yaml = "saga:\n  name: welcome\n  steps:\n    - task: hello\n"
    validate_saga_yaml(yaml)

def test_python_object_tag_blocked():
    yaml = "!!python/object/apply:os.system ['echo pwned']"
    with pytest.raises(YamlInvalidError):
        validate_saga_yaml(yaml)

def test_invalid_yaml():
    yaml = "name: 'foo"  # unclosed quote
    with pytest.raises(YamlInvalidError):
        validate_saga_yaml(yaml)
```

- [ ] **Step 2: Implement**

```python
# In template_validator.py add:
import yaml

class YamlInvalidError(Exception):
    pass

FORBIDDEN_TAGS = ["!!python/object", "!!python/name", "!!python/module", "!!python/tuple", "!!python/list"]

def validate_saga_yaml(yaml_text: str):
    try:
        loader = yaml.SafeLoader(yaml_text)
        # Block dangerous constructors
        def block_python(loader, suffix, node):
            raise YamlInvalidError(f"Forbidden YAML tag: !!python/{suffix}")
        for tag in ["object", "name", "module", "tuple", "list"]:
            yaml.SafeLoader.add_constructor(f"tag:yaml.org,2002:python/{tag}", block_python)
        loader.check_data()
    except yaml.YAMLError as e:
        raise YamlInvalidError(f"YAML parse error: {e}")
```

```python
# schemas/marketplace_saga.py
from pydantic import BaseModel, Field
from typing import Optional, List

class SagaTemplateCreate(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    icon_url: Optional[str] = None
    description: Optional[str] = None
    yaml_content: str = Field(min_length=1)
    npc_deps: Optional[List[str]] = []
    semantic_version: str = Field(pattern=r"^\d+\.\d+\.\d+$")
```

- [ ] **Step 3: Verify + Commit**

```bash
git commit -m "feat(market): Saga YAML sandbox + python tags blocked"
```

---

### Task 7: Saga template service + API

**Files:**
- Create: `apps/economy-service/src/economy_service/api/v1/marketplace/saga_templates.py`
- Modify: `apps/economy-service/src/economy_service/services/marketplace_service.py`
- Test: `apps/economy-service/tests/test_api_marketplace_saga.py`

- [ ] **Step 1: Service methods**

```python
# Add to marketplace_service.py:
async def create_saga_template(creator_id, name, yaml_content, semantic_version,
                               icon_url=None, description=None, npc_deps=None):
    conn = await get_conn()
    row = await conn.fetchrow(
        """
        INSERT INTO saga_template (creator_id, name, icon_url, description, yaml_content, npc_deps, semantic_version)
        VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id
        """,
        creator_id, name, icon_url, description, yaml_content, npc_deps or [], semantic_version,
    )
    return row["id"]

async def list_saga_templates(status="live", limit=50, offset=0):
    conn = await get_conn()
    rows = await conn.fetch(
        "SELECT * FROM saga_template WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
        status, limit, offset,
    )
    return [dict(r) for r in rows]
```

- [ ] **Step 2: API endpoints** (mirror npc_templates.py pattern)

```python
# apps/economy-service/src/economy_service/api/v1/marketplace/saga_templates.py
@router.post("")
async def create(body: SagaTemplateCreate, authorization: str = Header(...)):
    username = authorization.replace("Bearer ", "")
    await require_role(["creator", "admin"], username)
    try:
        validate_saga_yaml(body.yaml_content)
    except YamlInvalidError as e:
        raise HTTPException(status_code=400, detail={"code": "R_032", "msg": str(e)})
    tid = await svc.create_saga_template(
        creator_id=username, name=body.name, yaml_content=body.yaml_content,
        semantic_version=body.semantic_version, icon_url=body.icon_url,
        description=body.description, npc_deps=body.npc_deps,
    )
    return {"id": tid}

@router.get("")
async def list_(status="live", limit=50, offset=0):
    return await svc.list_saga_templates(status=status, limit=limit, offset=offset)

@router.post("/{tid}/take-down")
async def take_down(tid: int, authorization: str = Header(...)):
    username = authorization.replace("Bearer ", "")
    await require_role(["admin"], username)
    # ... similar to npc
```

- [ ] **Step 3: Tests + Verify + Commit**

```bash
git commit -m "feat(market): Saga template service + API + role check"
```

---

## W4 — Purchase + Revenue

### Task 8: Errors module updates (R_027~R_034)

**Files:**
- Modify: `apps/economy-service/src/economy_service/errors.py`

- [ ] **Step 1: Add new error codes**

```python
# In errors.py:
R_027_CREATOR_REQUIRED = ("R_027", "creator role required", 403)
R_028_TEMPLATE_NOT_FOUND = ("R_028", "template not found", 404)
R_029_TEMPLATE_TAKEN_DOWN = ("R_029", "template taken down", 410)
R_030_PRICE_INVALID = ("R_030", "price must be >= 10", 400)
R_031_BT_INVALID = ("R_031", "BT skeleton invalid", 400)
R_032_YAML_INVALID = ("R_032", "YAML content invalid", 400)
R_033_SELF_PURCHASE = ("R_033", "creator cannot purchase own template", 403)
R_034_PURCHASE_DUPLICATE = ("R_034", "purchase duplicate (idempotency)", 409)
```

- [ ] **Step 2: Commit**

```bash
git commit -m "feat(market): error codes R_027-R_034"
```

---

### Task 9: Purchase service (PG transaction + idempotency)

**Files:**
- Modify: `apps/economy-service/src/economy_service/services/marketplace_service.py`
- Test: `apps/economy-service/tests/test_marketplace_purchase_service.py`

- [ ] **Step 1: Write failing tests**

```python
@pytest.mark.asyncio
async def test_purchase_npc_template_deducts_and_pays():
    # setup: creator with 0 gold, buyer with 1000 gold, template price=100
    purchase_id = await purchase_template(
        user_id="buyer", template_kind="npc", template_id=42, idempotency_key="key-1"
    )
    assert purchase_id > 0
    buyer_balance = await get_balance("buyer")
    creator_balance = await get_balance("creator_demo")
    assert buyer_balance == 900
    assert creator_balance == 100

@pytest.mark.asyncio
async def test_purchase_idempotency():
    pid1 = await purchase_template("buyer", "npc", 42, idempotency_key="k1")
    pid3 = await purchase_template("buyer", "npc", 42, idempotency_key="k1")
    assert pid1 == pid3  # same purchase

@pytest.mark.asyncio
async def test_purchase_insufficient_balance_raises():
    with pytest.raises(InsufficientBalanceError):
        await purchase_template("poor", "npc", 42, idempotency_key="k2")
```

- [ ] **Step 2: Implement**

```python
# In marketplace_service.py:
import json
from economy_service.errors import (
    R_022_INSUFFICIENT_BALANCE,
    R_029_TEMPLATE_TAKEN_DOWN,
    R_028_TEMPLATE_NOT_FOUND,
    R_033_SELF_PURCHASE,
    R_034_PURCHASE_DUPLICATE,
)

class InsufficientBalanceError(Exception):
    pass

async def purchase_template(user_id, template_kind, template_id, idempotency_key):
    conn = await get_conn()
    async with conn.transaction():
        # 1. Idempotency check
        existing = await conn.fetchrow(
            "SELECT id FROM template_purchase WHERE idempotency_key = $1",
            idempotency_key,
        )
        if existing:
            return existing["id"]

        # 2. Load template + price
        if template_kind == "npc":
            row = await conn.fetchrow(
                "SELECT creator_id, price_gold, status FROM npc_template WHERE id = $1 FOR UPDATE",
                template_id,
            )
        else:
            row = await conn.fetchrow(
                "SELECT creator_id, 0 AS price_gold, status FROM saga_template WHERE id = $1 FOR UPDATE",
                template_id,
            )
        if not row:
            raise R_028_TEMPLATE_NOT_FOUND
        if row["status"] != "live":
            raise R_029_TEMPLATE_TAKEN_DOWN

        creator_id = row["creator_id"]
        price = row["price_gold"]

        # 2b. Block self-purchase
        if creator_id == user_id:
            raise R_033_SELF_PURCHASE

        # 3. Check buyer balance (use existing wallet_service)
        from economy_service.services.wallet_service import get_balance, deduct, add
        buyer_bal = await get_balance(user_id)
        if buyer_bal < price:
            raise R_022_INSUFFICIENT_BALANCE

        # 4. Insert purchase record (idempotency)
        try:
            purchase_id = await conn.fetchval(
                """
                INSERT INTO template_purchase (user_id, template_kind, template_id, price_paid_gold, idempotency_key)
                VALUES ($1, $2, $3, $4, $5) RETURNING id
                """,
                user_id, template_kind, template_id, price, idempotency_key,
            )
        except UniqueViolationError:
            existing = await conn.fetchrow(
                "SELECT id FROM template_purchase WHERE idempotency_key = $1",
                idempotency_key,
            )
            return existing["id"]

        # 5. Atomic transfer (deduct buyer + add creator)
        await deduct(user_id, price, tx_id=f"market:{purchase_id}")
        if price > 0:
            await add(creator_id, price, tx_id=f"market:{purchase_id}")

        # 6. Insert revenue ledger
        await conn.execute(
            """
            INSERT INTO creator_revenue (creator_id, purchase_id, amount_gold, platform_cut_gold)
            VALUES ($1, $2, $3, 0)
            """,
            creator_id, purchase_id, price,
        )

        # 7. Kafka emit (fire-and-forget)
        from economy_service.clients.kafka_producer import emit
        await emit("market.purchased", {
            "purchase_id": purchase_id,
            "user_id": user_id,
            "creator_id": creator_id,
            "template_kind": template_kind,
            "template_id": template_id,
            "price_paid_gold": price,
        })

        return purchase_id
```

- [ ] **Step 3: Verify + Commit**

```bash
git commit -m "feat(market): purchase service (transaction + idempotency + Kafka)"
```

---

### Task 10: Purchase API endpoint + inventory + revenue endpoints

**Files:**
- Create: `apps/economy-service/src/economy_service/api/v1/marketplace/purchase.py`
- Modify: `apps/economy-service/src/economy_service/app.py`

- [x] **Step 1: Implement**

```python
# apps/economy-service/src/economy_service/api/v1/marketplace/purchase.py
from fastapi import APIRouter, Header, HTTPException
from pydantic import BaseModel, Field
from economy_service.services import marketplace_service as svc

router = APIRouter(prefix="/v1/marketplace", tags=["market"])

class PurchaseRequest(BaseModel):
    template_kind: str = Field(pattern="^(npc|saga)$")
    template_id: int
    idempotency_key: str

@router.post("/purchase")
async def purchase(body: PurchaseRequest, authorization: str = Header(...)):
    username = authorization.replace("Bearer ", "")
    try:
        purchase_id = await svc.purchase_template(
            user_id=username,
            template_kind=body.template_kind,
            template_id=body.template_id,
            idempotency_key=body.idempotency_key,
        )
    except Exception as e:
        if e.__class__.__name__ == "R_022_INSUFFICIENT_BALANCE":
            raise HTTPException(status_code=402, detail={"code": "R_022", "msg": str(e)})
        if e.__class__.__name__ == "R_029_TEMPLATE_TAKEN_DOWN":
            raise HTTPException(status_code=410, detail={"code": "R_029", "msg": str(e)})
        raise
    return {"purchase_id": purchase_id}

@router.get("/inventory/{user_id}")
async def inventory(user_id: str):
    conn = await get_conn()
    rows = await conn.fetch(
        """
        SELECT p.id AS purchase_id, p.template_kind, p.template_id, p.price_paid_gold, p.created_at
        FROM template_purchase p WHERE p.user_id = $1 ORDER BY p.created_at DESC
        """,
        user_id,
    )
    return [dict(r) for r in rows]

@router.get("/revenue/{creator_id}")
async def revenue(creator_id: str, authorization: str = Header(...)):
    username = authorization.replace("Bearer ", "")
    await require_role(["creator", "admin"], creator_id)
    conn = await get_conn()
    rows = await conn.fetch(
        "SELECT * FROM creator_revenue WHERE creator_id = $1 ORDER BY created_at DESC",
        creator_id,
    )
    return [dict(r) for r in rows]
```

- [x] **Step 2: Tests + verify + commit**

```bash
git commit -m "feat(market): purchase + inventory + revenue API endpoints"
```

---

## W5 — Admin-portal JWT role + creator UI for NPC

### Task 11: admin-portal JWT payload role claim

**Files:**
- Modify: `apps/admin-portal/src/lib/auth.ts`
- Test: `apps/admin-portal/tests/lib/auth.test.ts`

- [x] **Step 1: Write failing test**

```typescript
import { decodeToken } from '@/lib/auth';
const token = 'eyJ...valid_payload_with_role_creator...';
const session = decodeToken(token, 'secret');
expect(session.role).toBe('creator');
```

- [x] **Step 2: Update decodeToken**

```typescript
// In apps/admin-portal/src/lib/auth.ts — add role field
export interface Session {
  username: string;
  role: 'player' | 'creator' | 'admin';
  exp: number;
}

export function decodeToken(token: string, secret: string): Session | null {
  // ... existing decode logic, parse payload.role
  return {
    username: payload.username,
    role: payload.role ?? 'player',
    exp: payload.exp,
  };
}
```

- [x] **Step 3: Tests + verify + commit**

```bash
git commit -m "feat(admin-portal): JWT decode role claim (player/creator/admin)"
```

---

### Task 12: admin-portal marketplace proxy helpers

**Files:**
- Create: `apps/admin-portal/src/lib/marketplace.ts`
- Create: `apps/admin-portal/src/app/api/marketplace/npc-templates/route.ts`
- Create: `apps/admin-portal/src/app/api/marketplace/saga-templates/route.ts`
- Create: `apps/admin-portal/src/app/api/marketplace/purchase/route.ts`
- Create: `apps/admin-portal/src/app/api/marketplace/inventory/[user_id]/route.ts`
- Create: `apps/admin-portal/src/app/api/marketplace/revenue/[creator_id]/route.ts`

- [x] **Step 1: marketplace.ts helper**

```typescript
// apps/admin-portal/src/lib/marketplace.ts
const ECONOMY_URL = process.env.ADMIN_PORTAL_ECONOMY_URL ?? 'http://economy-service:8005';

export async function proxyMarketplace(path: string, init?: RequestInit) {
  const r = await fetch(`${ECONOMY_URL}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });
  if (!r.ok) {
    const err = await r.json().catch(() => ({}));
    throw Object.assign(new Error('marketplace error'), { status: r.status, body: err });
  }
  return r.json();
}
```

- [x] **Step 2: 5 proxy routes** (each forwards to economy-service)

For each proxy, follow the established pattern from T3-T5 (proxyGet/proxyPost + JWT from cookie). Example:

```typescript
// apps/admin-portal/src/app/api/marketplace/npc-templates/route.ts
import { NextRequest, NextResponse } from 'next/server';
import { cookies } from 'next/headers';
import { proxyMarketplace } from '@/lib/marketplace';
import { decodeToken, COOKIE_NAME, getJwtSecret } from '@/lib/auth';

export async function GET(req: NextRequest) {
  const token = cookies().get(COOKIE_NAME)?.value;
  const session = token ? decodeToken(token, getJwtSecret()) : null;
  if (!session || session.role === 'player') {
    return NextResponse.json({ error: { code: 'R_027', msg: '...' } }, { status: 403 });
  }
  const items = await proxyMarketplace('/v1/marketplace/npc-templates');
  return NextResponse.json(items);
}

export async function POST(req: NextRequest) {
  const token = cookies().get(COOKIE_NAME)?.value;
  const session = token ? decodeToken(token, getJwtSecret()) : null;
  if (!session || !['creator', 'admin'].includes(session.role)) {
    return NextResponse.json({ error: { code: 'R_027', msg: '...' } }, { status: 403 });
  }
  const body = await req.json();
  const result = await proxyMarketplace('/v1/marketplace/npc-templates', {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${session.username}` },
    body: JSON.stringify(body),
  });
  return NextResponse.json(result);
}
```

- [x] **Step 3: Tests + verify + commit**

```bash
git commit -m "feat(admin-portal): marketplace proxy routes (npc/saga/purchase/inventory/revenue)"
```

---

### Task 13: /creator/npc-templates list page

**Files:**
- Create: `apps/admin-portal/src/app/creator/npc-templates/page.tsx`
- Create: `apps/admin-portal/src/app/creator/npc-templates/NpcTemplateList.tsx`
- Test: `apps/admin-portal/tests/creator/npc-template-list.test.tsx`

- [x] **Step 1: Write failing test**

```typescript
// Renders list of templates + "创建" button when role=creator
// Uses existing renderWithQuery pattern
```

- [x] **Step 2: Implement NpcTemplateList + page.tsx** (thin wrapper)

Use WalletClient pattern: `'use client'`, useQuery, describeError, empty/loading/error states.

- [x] **Step 3: Verify + commit**

```bash
git commit -m "feat(admin-portal): /creator/npc-templates list page + 2 tests"
```

---

### Task 14: /creator/npc-templates/new edit form

**Files:**
- Create: `apps/admin-portal/src/app/creator/npc-templates/new/page.tsx`
- Create: `apps/admin-portal/src/app/creator/npc-templates/new/NpcTemplateEdit.tsx`
- Test: `apps/admin-portal/tests/creator/npc-template-edit.test.tsx`

- [x] **Step 1: Write failing test**

- [x] **Step 2: Implement form** (OCEAN sliders + BT textarea + 商品 list + price)

- [x] **Step 3: Verify + commit**

```bash
git commit -m "feat(admin-portal): /creator/npc-templates/new edit form"
```

---

## W6 — Creator UI for Saga

### Task 15: /creator/saga-templates list + edit

**Files:**
- Create: `apps/admin-portal/src/app/creator/saga-templates/page.tsx`
- Create: `apps/admin-portal/src/app/creator/saga-templates/SagaTemplateList.tsx`
- Create: `apps/admin-portal/src/app/creator/saga-templates/new/page.tsx`
- Create: `apps/admin-portal/src/app/creator/saga-templates/new/SagaTemplateEdit.tsx`
- Test: `apps/admin-portal/tests/creator/saga-template.test.tsx`

- [x] **Step 1: Write failing test (list + edit)**

```typescript
import { renderWithQuery } from '@/tests/test-utils';
import SagaTemplateList from '@/app/creator/saga-templates/SagaTemplateList';
import SagaTemplateEdit from '@/app/creator/saga-templates/new/SagaTemplateEdit';
import { screen, waitFor } from '@testing-library/react';

test('renders saga template list from API', async () => {
  renderWithQuery(<SagaTemplateList />);
  await waitFor(() => expect(screen.getByText('WelcomeSaga')).toBeInTheDocument());
});

test('edit form has yaml textarea + NPC deps + version', () => {
  renderWithQuery(<SagaTemplateEdit />);
  expect(screen.getByLabelText(/YAML 内容/)).toBeInTheDocument();
  expect(screen.getByLabelText(/依赖 NPC/)).toBeInTheDocument();
  expect(screen.getByLabelText(/语义化版本/)).toBeInTheDocument();
});
```

- [x] **Step 2: SagaTemplateList.tsx**

```tsx
'use client';
import { useQuery } from '@tanstack/react-query';
import { describeError } from '@/lib/errors';

export default function SagaTemplateList() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['saga-templates', 'mine'],
    queryFn: () => fetch('/api/marketplace/saga-templates').then(r => r.json()),
  });
  if (isLoading) return <div data-testid="loading">加载中...</div>;
  if (error) return <div data-testid="error">{describeError(error)}</div>;
  return (
    <div data-testid="saga-template-list">
      <h1>Saga 模板</h1>
      <a href="/creator/saga-templates/new" data-testid="create-saga-template">+ 新建 Saga</a>
      <table>
        <thead><tr><th>名称</th><th>版本</th><th>状态</th><th>创建时间</th></tr></thead>
        <tbody>
          {(data ?? []).map((t: any) => (
            <tr key={t.id}><td>{t.name}</td><td>{t.semantic_version}</td>
                <td>{t.status}</td><td>{new Date(t.created_at).toLocaleString()}</td></tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

```tsx
// apps/admin-portal/src/app/creator/saga-templates/page.tsx
import SagaTemplateList from './SagaTemplateList';
export default function Page() { return <SagaTemplateList />; }
```

- [x] **Step 3: SagaTemplateEdit.tsx**

```tsx
'use client';
import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { describeError } from '@/lib/errors';

export default function SagaTemplateEdit() {
  const router = useRouter();
  const qc = useQueryClient();
  const [name, setName] = useState('');
  const [yamlContent, setYamlContent] = useState('saga:\n  name: my-saga\n  steps:\n    - task: hello\n');
  const [version, setVersion] = useState('1.0.0');
  const [npcDeps, setNpcDeps] = useState('');

  const create = useMutation({
    mutationFn: () => fetch('/api/marketplace/saga-templates', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name, yaml_content: yamlContent, semantic_version: version,
        npc_deps: npcDeps.split(',').map(s => s.trim()).filter(Boolean),
      }),
    }).then(async r => {
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).msg ?? 'create failed');
      return r.json();
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['saga-templates', 'mine'] });
      router.push('/creator/saga-templates');
    },
  });

  return (
    <div data-testid="saga-template-edit">
      <h1>新建 Saga 模板</h1>
      <label>名称<input value={name} onChange={e => setName(e.target.value)} /></label>
      <label>YAML 内容<textarea value={yamlContent} onChange={e => setYamlContent(e.target.value)} rows={10} /></label>
      <label>依赖 NPC (npc_template id, 逗号分隔)<input value={npcDeps} onChange={e => setNpcDeps(e.target.value)} placeholder="1,2" /></label>
      <label>语义化版本<input value={version} onChange={e => setVersion(e.target.value)} /></label>
      <button onClick={() => create.mutate()} disabled={!name || create.isPending} data-testid="save-saga">
        {create.isPending ? '保存中…' : '保存'}
      </button>
      {create.error && <div data-testid="error">{describeError(create.error)}</div>}
    </div>
  );
}
```

```tsx
// apps/admin-portal/src/app/creator/saga-templates/new/page.tsx
import SagaTemplateEdit from './SagaTemplateEdit';
export default function Page() { return <SagaTemplateEdit />; }
```

- [x] **Step 4: Verify tests + commit**

```bash
cd apps/admin-portal && pnpm exec vitest run tests/creator/saga-template.test.tsx
cd apps/admin-portal && pnpm exec tsc --noEmit
git add apps/admin-portal/src/app/creator/saga-templates/ \
        apps/admin-portal/tests/creator/saga-template.test.tsx
git commit -m "feat(admin-portal): /creator/saga-templates list + edit form"
```

---

## W7 — Marketplace + Inventory UI + E2E

### Task 16: /market browse page

**Files:**
- Create: `apps/admin-portal/src/app/market/page.tsx` + `MarketClient.tsx`
- Test: `apps/admin-portal/tests/market.test.tsx`

- [x] **Step 1-3:** Tab 切换 NPC / Saga + 卡片网格 + 搜索

```bash
git commit -m "feat(admin-portal): /market browse page (NPC + Saga tabs)"
```

---

### Task 17: /market/npc-templates/{id} detail

**Files:**
- Create: `apps/admin-portal/src/app/market/npc-templates/[id]/page.tsx` + `NpcTemplateDetail.tsx`
- Test: `apps/admin-portal/tests/market/npc-detail.test.tsx`

- [x] **Step 1-3:** OCEAN radar chart SVG + 商品 list + 购买按钮

```bash
git commit -m "feat(admin-portal): /market/npc-templates/{id} detail page"
```

---

### Task 18: /market/saga-templates/{id} detail

**Files:**
- Create: `apps/admin-portal/src/app/market/saga-templates/[id]/page.tsx` + `SagaTemplateDetail.tsx`
- Test: `apps/admin-portal/tests/market/saga-detail.test.tsx`

- [x] **Step 1-3:** yaml preview + NPC deps list + 购买按钮

```bash
git commit -m "feat(admin-portal): /market/saga-templates/{id} detail page"
```

---

### Task 19: /inventory page

**Files:**
- Create: `apps/admin-portal/src/app/inventory/page.tsx` + `InventoryClient.tsx`
- Test: `apps/admin-portal/tests/inventory.test.tsx`

- [x] **Step 1-3:** 列出 template_purchase

```bash
git commit -m "feat(admin-portal): /inventory page (purchased templates)"
```

---

### Task 20: Sidebar updates + take-down OIDC

**Files:**
- Modify: `apps/admin-portal/src/components/Sidebar.tsx`
- Create: `apps/admin-portal/src/app/api/marketplace/npc-templates/[id]/take-down/route.ts`
- Create: `apps/admin-portal/src/app/api/marketplace/saga-templates/[id]/take-down/route.ts`

- [x] **Step 1:** Add Creator / Market / Inventory links to Sidebar

- [x] **Step 2:** 2 take-down proxy routes (admin role check)

```bash
git commit -m "feat(admin-portal): Sidebar adds Creator/Market/Inventory + take-down proxies"
```

---

### Task 21: Playwright E2E test

**Files:**
- Create: `apps/admin-portal/e2e/creator-market.spec.ts`

- [x] **Step 1: Write the spec** — login as creator → create template → login as buyer → buy → assert creator revenue increased

- [x] **Step 2: Verify `--list` + commit**

```bash
git commit -m "test(admin-portal): Playwright E2E for creator marketplace"
```

---

## W8 — Acceptance + E2E + Docs

### Task 22: 3 e2e integration tests

**Files:**
- Create: `apps/economy-service/tests/test_e2e_npc_purchase.py`
- Create: `apps/economy-service/tests/test_e2e_saga_purchase.py`
- Create: `apps/economy-service/tests/test_e2e_takedown.py`

- [x] **Step 1-3:** Per spec §测试策略 — full flow tests

```bash
git commit -m "test(market): e2e tests for NPC purchase + Saga deps + take-down"
```

---

### Task 23: acceptance_creator_market_v1.py (10 步)

**Files:**
- Create: `apps/economy-service/scripts/acceptance_creator_market_v1.py`

- [x] **Step 1:** Follow pattern of `acceptance_economy_v1.py` (existing 3.0 v1) — 10 steps per spec

- [x] **Step 2: Run + verify 10/10 PASS**

```bash
docker compose exec -T economy-service python scripts/acceptance_creator_market_v1.py
```

- [x] **Step 3: Commit**

```bash
git commit -m "test(market): acceptance_creator_market_v1.py (10 steps PASS)"
```

---

### Task 24: ADR-0010 + ROADMAP + CHANGELOG

**Files:**
- Create: `docs/adr/ADR-0010-creator-marketplace.md`
- Modify: `docs/3.0-ROADMAP.md`
- Modify: `CHANGELOG-3.0.md`

- [x] **Step 1: ADR-0010** — follow ADR-0009 structure (293 lines), document the 8 design decisions + 3-role identity + 即时分账 + platform_npc_pool Saga deps

- [x] **Step 2: ROADMAP** — add §3.0 v3 创作者市场 (GA 2026-XX-XX)

- [x] **Step 3: CHANGELOG** — prepend v3 entry

- [x] **Step 4: Commit**

```bash
git commit -m "docs(3.0 v3): ADR-0010 + ROADMAP v3 section + CHANGELOG entry"
```

---

### Task 25: Final smoke — docker compose + full test suite

- [x] **Step 1:** Run `docker compose up -d --build` — all containers healthy

- [x] **Step 2:** `cd apps/economy-service && PYTHONPATH=src pytest tests/ -v` — 30+ unit + 8+ e2e pass

- [x] **Step 3:** `cd apps/admin-portal && pnpm exec vitest run` — 9+ vitest pass

- [x] **Step 4:** `cd apps/admin-portal && pnpm exec tsc --noEmit` — 0 errors

- [x] **Step 5:** `cd apps/admin-portal && pnpm exec playwright test e2e/creator-market.spec.ts` — 1 pass

- [x] **Step 6:** `docker compose exec -T economy-service python scripts/acceptance_creator_market_v1.py` — 10/10 PASS

---

## Verification (DoD)

1. ✅ 30+ unit + 8+ e2e tests (pytest)
2. ✅ 9+ vitest + 1 Playwright
3. ✅ acceptance 10/10
4. ✅ docker compose healthy
5. ✅ tsc clean
6. ✅ Manual: creator publish → market visible → gold transfer → takedown → saga deps resolve
