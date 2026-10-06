"""R_016 / R_017 跨城流式错误码 + HTTP 状态映射。"""
import pytest
from agent_os.errors import (
    R016CrossCityStreamFail,
    R017CrossCityStreamTimeout,
    AgentOSError,
)


def test_r016_attributes():
    err = R016CrossCityStreamFail()
    assert err.code == "R_016"
    assert err.http_status == 502
    assert "跨城" in err.message or "流式" in err.message


def test_r017_attributes():
    err = R017CrossCityStreamTimeout()
    assert err.code == "R_017"
    assert err.http_status == 504
    assert "跨城" in err.message or "流式" in err.message


def test_r016_inherits_agent_os_error():
    err = R016CrossCityStreamFail("upstream dial fail")
    assert isinstance(err, AgentOSError)
    assert "upstream dial fail" in str(err)


def test_r017_inherits_agent_os_error():
    err = R017CrossCityStreamTimeout("upstream 5s timeout")
    assert isinstance(err, AgentOSError)


def test_codes_unique_among_all():
    """R_016 / R_017 不与既有 1..15 重复。"""
    import agent_os.errors as errs
    codes = {v.code for k, v in vars(errs).items()
             if isinstance(v, type) and issubclass(v, AgentOSError) and v is not AgentOSError}
    assert "R_016" in codes
    assert "R_017" in codes
    # Total >= 17 (R_001..R_017)
    r_codes = {c for c in codes if c.startswith("R_")}
    assert len(r_codes) >= 17


def test_http_status_distinct_for_each():
    """R_016 (502) vs R_017 (504) 状态码不同（防 typo）。"""
    assert R016CrossCityStreamFail.http_status != R017CrossCityStreamTimeout.http_status
    assert R016CrossCityStreamFail.http_status == 502
    assert R017CrossCityStreamTimeout.http_status == 504