"""flush() + generator state-safety tests (added after T01 review)."""
import pytest
from agent_os.stream.sentence_splitter import SentenceSplitter


def test_flush_empty_buffer():
    s = SentenceSplitter()
    assert list(s.flush()) == []


def test_flush_whitespace_only():
    s = SentenceSplitter()
    s._buffer = "   \n  "
    assert list(s.flush()) == []


def test_flush_orphan_partial_tag_stripped():
    """流中断在 <emotion=happy> 中间 → 不应泄漏 raw tag。"""
    s = SentenceSplitter()
    s._buffer = "<emotion=happy>来了您嘞"
    out = list(s.flush())
    assert out == [("来了您嘞", "neutral")]


def test_flush_complete_sentence_yields_as_neutral():
    s = SentenceSplitter()
    s._buffer = "完整句子但没 emotion tag"
    out = list(s.flush())
    assert out == [("完整句子但没 emotion tag", "neutral")]


def test_consumer_break_does_not_re_emit():
    """Critical bug regression: consumer break mid-iteration → next call 不重发。"""
    s = SentenceSplitter()
    chunks = [
        "<emotion=happy>第一句。</emotion>",
        "<emotion=neutral>第二句。</emotion>",
    ]
    gen = s.feed(chunks)
    first = next(gen)  # 只消费第一个
    assert first == ("第一句。", "happy")
    # 关键：不消费剩余的，下一次 feed 不应重发 "第一句。"
    more = list(s.feed([]))
    assert more == []  # buffer 已清，无重发


def test_feed_after_done_is_noop():
    """<end> 后再 feed → 应该 noop（hard stop）。"""
    s = SentenceSplitter()
    list(s.feed(["<emotion=happy>第一。</emotion>", "<end>"]))
    more = list(s.feed(["<emotion=sad>第二。</emotion>"]))
    assert more == []


def test_double_finditer_collapsed():
    """验证单 pass（不是双 finditer）—— 通过行为测试。"""
    s = SentenceSplitter()
    chunks = ["<emotion=happy>第一。</emotion>", "<emotion=neutral>第二。</emotion>"]
    out = list(s.feed(chunks))
    assert out == [("第一。", "happy"), ("第二。", "neutral")]
    assert s._buffer == ""  # buffer 完全清空
