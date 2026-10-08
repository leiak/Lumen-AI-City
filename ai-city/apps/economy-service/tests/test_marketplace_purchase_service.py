import pytest
from economy_service.errors import (
    InsufficientBalance,
    SelfPurchaseError,
    TemplateTakenDownError,
)
from economy_service.services import marketplace_service
from economy_service.services.marketplace_service import MarketplaceService

BUYER_ID = "11111111-1111-1111-1111-111111111111"
CREATOR_ID = "22222222-2222-2222-2222-222222222222"
IDEMPOTENCY_KEY = "purchase-key-1"


class FakeTransaction:
    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc):
        return False


class FakeConnection:
    def __init__(self, templates, wallets):
        self.templates = templates
        self.wallets = wallets
        self.purchases = {}
        self.revenues = []
        self.logs = []
        self.next_purchase_id = 71

    async def fetchrow(self, sql, *params):
        if "FROM template_purchase WHERE idempotency_key" in sql:
            return self.purchases.get(params[0])
        if "FROM npc_template WHERE id" in sql or "FROM saga_template WHERE id" in sql:
            table = sql.split("FROM ")[1].split(" ")[0]
            return self.templates[(table, params[0])]
        if "INSERT INTO template_purchase" in sql:
            purchase_id = self.next_purchase_id
            self.next_purchase_id += 1
            self.purchases[params[4]] = {"id": purchase_id}
            return {"id": purchase_id}
        if "FROM wallet WHERE user_id" in sql:
            if params[0] not in self.wallets:
                return None
            return {"gold_balance": self.wallets[params[0]], "token_balance": 0}
        return None

    async def execute(self, sql, *params):
        if "UPDATE wallet SET gold_balance" in sql:
            self.wallets[params[1]] = params[0]
        elif "INSERT INTO creator_revenue" in sql:
            self.revenues.append(params)
        elif "INSERT INTO transaction" in sql:
            self.logs.append(params)

    async def fetch(self, sql, *params):
        self.last_fetch = (sql, params)
        return [{"purchase_id": 71, "amount_gold": 100}]

    def transaction(self):
        return FakeTransaction()


class FakePool:
    def __init__(self, conn):
        self._conn = conn

    def acquire(self):
        conn = self._conn

        class _ConnContext:
            async def __aenter__(self):
                return conn

            async def __aexit__(self, *exc):
                return False

        return _ConnContext()


def make_pool(buyer_gold=1000, creator_gold=0, status="live"):
    templates = {
        ("npc_template", 42): {"creator_id": CREATOR_ID, "price_gold": 100, "status": status},
        ("saga_template", 43): {"creator_id": CREATOR_ID, "price_gold": 100, "status": status},
    }
    wallets = {BUYER_ID: buyer_gold, CREATOR_ID: creator_gold}
    conn = FakeConnection(templates, wallets)
    return FakePool(conn), conn


@pytest.fixture(autouse=True)
def reset_marketplace_clients():
    marketplace_service.set_clients(kafka=None)
    yield
    marketplace_service.set_clients(kafka=None)


@pytest.mark.asyncio
@pytest.mark.parametrize("template_kind,template_id", [("npc", 42), ("saga", 43)])
async def test_purchase_transfers_gold_and_records_revenue(template_kind, template_id):
    pool, conn = make_pool()
    service = MarketplaceService(pool)

    purchase_id = await service.purchase_template(
        user_id=BUYER_ID,
        template_kind=template_kind,
        template_id=template_id,
        idempotency_key=IDEMPOTENCY_KEY,
    )

    assert purchase_id == 71
    assert conn.wallets[BUYER_ID] == 900
    assert conn.wallets[CREATOR_ID] == 100
    assert conn.revenues == [(CREATOR_ID, 71, 100)]
    assert len(conn.logs) == 2
    assert conn.purchases[IDEMPOTENCY_KEY] == {"id": 71}


@pytest.mark.asyncio
async def test_purchase_is_idempotent():
    pool, conn = make_pool()
    service = MarketplaceService(pool)

    first = await service.purchase_template(
        user_id=BUYER_ID, template_kind="npc", template_id=42, idempotency_key=IDEMPOTENCY_KEY
    )
    second = await service.purchase_template(
        user_id=BUYER_ID, template_kind="npc", template_id=42, idempotency_key=IDEMPOTENCY_KEY
    )

    assert first == second == 71
    assert conn.wallets[BUYER_ID] == 900
    assert conn.wallets[CREATOR_ID] == 100
    assert len(conn.revenues) == 1


@pytest.mark.asyncio
@pytest.mark.parametrize("template_kind,template_id", [("npc", 42), ("saga", 43)])
async def test_purchase_insufficient_balance(template_kind, template_id):
    pool, conn = make_pool(buyer_gold=9)
    service = MarketplaceService(pool)

    with pytest.raises(InsufficientBalance):
        await service.purchase_template(
            user_id=BUYER_ID, template_kind=template_kind, template_id=template_id,
            idempotency_key=IDEMPOTENCY_KEY,
        )

    assert conn.revenues == []


@pytest.mark.asyncio
async def test_purchase_rejects_taken_down_template():
    pool, conn = make_pool(status="taken_down")
    service = MarketplaceService(pool)

    with pytest.raises(TemplateTakenDownError):
        await service.purchase_template(
            user_id=BUYER_ID, template_kind="npc", template_id=42,
            idempotency_key=IDEMPOTENCY_KEY,
        )

    assert conn.revenues == []


@pytest.mark.asyncio
async def test_purchase_rejects_self_purchase():
    pool, conn = make_pool()
    service = MarketplaceService(pool)

    with pytest.raises(SelfPurchaseError):
        await service.purchase_template(
            user_id=CREATOR_ID, template_kind="npc", template_id=42,
            idempotency_key=IDEMPOTENCY_KEY,
        )

    assert conn.revenues == []


@pytest.mark.asyncio
async def test_purchase_sends_event_after_commit():
    pool, _ = make_pool()

    class FakeKafka:
        def __init__(self):
            self.sent = []

        async def send(self, topic, payload):
            self.sent.append((topic, payload))

    kafka = FakeKafka()
    marketplace_service.set_clients(kafka=kafka)
    service = MarketplaceService(pool)

    await service.purchase_template(
        user_id=BUYER_ID, template_kind="npc", template_id=42,
        idempotency_key=IDEMPOTENCY_KEY,
    )

    assert kafka.sent == [(
        "econ.market.purchased",
        {
            "purchase_id": 71,
            "user_id": BUYER_ID,
            "creator_id": CREATOR_ID,
            "template_kind": "npc",
            "template_id": 42,
            "price_paid_gold": 100,
        },
    )]


@pytest.mark.asyncio
async def test_list_inventory_queries_owned_purchases():
    pool, conn = make_pool()
    service = MarketplaceService(pool)

    items = await service.list_inventory(BUYER_ID, limit=20, offset=5)

    assert items == [{"purchase_id": 71, "amount_gold": 100}]
    sql, params = conn.last_fetch
    assert "FROM template_purchase" in sql
    assert "WHERE p.user_id = $1" in sql
    assert tuple(params) == (BUYER_ID, 20, 5)


@pytest.mark.asyncio
async def test_list_creator_revenue_queries_creator_ledger():
    pool, conn = make_pool()
    service = MarketplaceService(pool)

    items = await service.list_creator_revenue(CREATOR_ID, limit=10, offset=2)

    assert items == [{"purchase_id": 71, "amount_gold": 100}]
    sql, params = conn.last_fetch
    assert "FROM creator_revenue" in sql
    assert "WHERE r.creator_id = $1" in sql
    assert tuple(params) == (CREATOR_ID, 10, 2)
