"""EmotionValidator：8 类 emotion 验证 + 异常降级 neutral。"""
import pytest
from agent_os.stream.emotion_validator import EmotionValidator, ALLOWED_EMOTIONS


def test_allowed_emotions_count():
    assert len(ALLOWED_EMOTIONS) == 8


def test_validate_8_emotions():
    v = EmotionValidator()
    for emo in ALLOWED_EMOTIONS:
        assert v.validate(emo) == emo


def test_validate_unknown_emotion_returns_neutral():
    v = EmotionValidator()
    assert v.validate("lol") == "neutral"
    assert v.validate("happy_face") == "neutral"


def test_validate_empty_returns_neutral():
    v = EmotionValidator()
    assert v.validate("") == "neutral"
    assert v.validate(None) == "neutral"


def test_validate_case_sensitive():
    """emotion 字符串必须小写（spec §2.3 LLM 输出约定）。"""
    v = EmotionValidator()
    assert v.validate("Happy") == "neutral"  # 大写不接受


def test_validate_whitespace_only_returns_neutral():
    """空白字符串视为非法 emotion（spec §2.3 LLM 输出约定）。"""
    v = EmotionValidator()
    assert v.validate("   ") == "neutral"
    assert v.validate("\t\n") == "neutral"


def test_validate_non_string_returns_neutral():
    """非字符串输入（int/list）降级 neutral，防御 LLM 输出异常。"""
    v = EmotionValidator()
    assert v.validate(42) == "neutral"
    assert v.validate([]) == "neutral"