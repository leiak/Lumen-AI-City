"""PlayerListener 纯逻辑：parse / 位置表 / 同 tile / welcome 去重。"""
from agent_os.player_listener import PlayerListener, PlayerPosition, parse_player_moved


def _pos(pid="p1", tile="tile_0_0", x=50.0, y=50.0, ts=1):
    return PlayerPosition(player_id=pid, tile_id=tile, x=x, y=y, ts_ms=ts)


def test_parse_player_moved_valid():
    p = parse_player_moved(
        '{"player_id":"p1","tile_id":"tile_0_0","x":12.5,"y":34.0,"ts_ms":1700000000}'
    )
    assert p is not None
    assert p.player_id == "p1"
    assert p.tile_id == "tile_0_0"
    assert p.x == 12.5
    assert p.y == 34.0
    assert p.ts_ms == 1700000000


def test_parse_player_moved_invalid():
    assert parse_player_moved("not json") is None
    assert parse_player_moved('{"player_id":"p1"}') is None
    assert parse_player_moved('{"player_id":1,"tile_id":"t","x":"a","y":0,"ts_ms":1}') is None


def test_update_position_overwrites():
    listener = PlayerListener()
    listener.update_position(_pos("p1", tile="tile_0_0", x=10))
    listener.update_position(_pos("p1", tile="tile_1_0", x=120))
    assert len(listener.players) == 1
    assert listener.players["p1"].tile_id == "tile_1_0"


def test_players_in_tile_exact():
    listener = PlayerListener()
    listener.update_position(_pos("p1", tile="tile_0_0"))
    listener.update_position(_pos("p2", tile="tile_1_0", x=120))
    result = listener.players_in_tile("tile_0_0")
    assert [p.player_id for p in result] == ["p1"]


def test_players_in_tile_no_match():
    listener = PlayerListener()
    listener.update_position(_pos("p1", tile="tile_2_3"))
    assert listener.players_in_tile("tile_0_0") == []


def test_is_in_range_same_tile():
    listener = PlayerListener()
    p = _pos("p1", tile="tile_0_0", x=50, y=50)
    assert listener.is_in_range(p, 60, 60) is True
    assert listener.is_in_range(p, 200, 50) is False


def test_welcomed_set():
    listener = PlayerListener()
    assert listener.is_welcomed("p1") is False
    listener.mark_welcomed("p1")
    assert listener.is_welcomed("p1") is True
    listener.clear_welcomed()
    assert listener.is_welcomed("p1") is False