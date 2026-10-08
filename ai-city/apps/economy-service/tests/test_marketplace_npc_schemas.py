import pytest
from economy_service.schemas.marketplace_npc import NpcTemplateCreate
from pydantic import ValidationError


def test_npc_template_create_valid():
    template = NpcTemplateCreate(
        name="Chef Wang",
        ocean_json={"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3},
        price_gold=100,
    )

    assert template.name == "Chef Wang"
    assert template.price_gold == 100


def test_npc_template_rejects_price_below_minimum():
    with pytest.raises(ValidationError, match="greater_than_equal"):
        NpcTemplateCreate(
            name="Too Cheap",
            ocean_json={"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
            price_gold=9,
        )


def test_npc_template_rejects_ocean_value_out_of_range():
    with pytest.raises(ValidationError, match="less_than_equal"):
        NpcTemplateCreate(
            name="Invalid Personality",
            ocean_json={"O": 1.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
            price_gold=10,
        )
