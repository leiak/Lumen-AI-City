import pytest
from economy_service.services.template_validator import BTInvalidError, validate_bt_skeleton


def test_accepts_valid_bt():
    bt = '{"type":"sequence","children":[{"type":"action","name":"square"}]}'

    validate_bt_skeleton(bt)


def test_rejects_bt_deeper_than_10():
    def sequence(child: str) -> str:
        return '{"type":"sequence","children":[' + child + ']}'

    bt = '{"type":"action","name":"leaf"}'
    for _ in range(11):
        bt = sequence(bt)

    with pytest.raises(BTInvalidError, match="depth"):
        validate_bt_skeleton(bt)


def test_rejects_bt_with_more_than_50_nodes():
    children = ",".join(['{"type":"action","name":"x"}'] * 51)
    bt = '{"type":"sequence","children":[' + children + ']}'

    with pytest.raises(BTInvalidError, match="50 nodes"):
        validate_bt_skeleton(bt)


def test_rejects_invalid_json():
    with pytest.raises(BTInvalidError, match="not valid JSON"):
        validate_bt_skeleton("{")
