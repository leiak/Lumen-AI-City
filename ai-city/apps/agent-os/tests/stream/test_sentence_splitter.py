"""SentenceSplitter 把 LLM 流式 token 切成 (text, emotion) 元组。"""
import pytest
from agent_os.stream.sentence_splitter import SentenceSplitter


@pytest.fixture
def splitter() -> SentenceSplitter:
    return SentenceSplitter()


def test_greeting_single_sentence(splitter):
    """单个完整句 + emotion tag → 一个元组。"""
    chunks = ["<emotion=happy>来了您嘞！</emotion>"]
    out = list(splitter.feed(chunks))
    assert out == [("来了您嘞！", "happy")]


def test_multiple_sentences_split(splitter):
    """3 个完整句 → 3 个元组。"""
    chunks = [
        "<emotion=happy>来了您嘞！</emotion>",
        "<emotion=neutral>几位？</emotion>",
        "<emotion=happy>坐坐，马上给您沏茶。</emotion>",
    ]
    out = list(splitter.feed(chunks))
    assert out == [
        ("来了您嘞！", "happy"),
        ("几位？", "neutral"),
        ("坐坐，马上给您沏茶。", "happy"),
    ]


def test_split_across_chunks(splitter):
    """句子跨越多个 token chunk → 仍正确切出。"""
    chunks = ["<emotion=happy>来了您嘞", "！</emotion>", "<emotion=neutral>几位？</emotion>"]
    out = list(splitter.feed(chunks))
    assert out == [("来了您嘞！", "happy"), ("几位？", "neutral")]


def test_sentence_end_punctuation(splitter):
    """句末标点（。！？~）切句。"""
    chunks = [
        "<emotion=neutral>今天天气真好。</emotion>",
        "<emotion=surprised>什么？！</emotion>",
        "<emotion=curious>去哪儿~</emotion>",
    ]
    out = list(splitter.feed(chunks))
    assert out == [
        ("今天天气真好。", "neutral"),
        ("什么？！", "surprised"),
        ("去哪儿~", "curious"),
    ]


def test_incomplete_buffer_does_not_emit(splitter):
    """未闭合的 tag 不输出元组。"""
    chunks = ["<emotion=happy>来了您嘞"]
    out = list(splitter.feed(chunks))
    assert out == []


def test_end_marker_triggers_done(splitter):
    """<end> tag 触发 done=True。"""
    chunks = [
        "<emotion=happy>来了您嘞！</emotion>",
        "<end>",
    ]
    out = list(splitter.feed(chunks))
    assert out == [("来了您嘞！", "happy")]
    assert splitter.done is True


def test_8_emotions_all_accepted(splitter):
    """8 类 emotion 全部正常解析。"""
    for emo in ["happy", "sad", "angry", "surprised", "thinking", "embarrassed", "curious", "neutral"]:
        chunks = [f"<emotion={emo}>测试文本。</emotion>"]
        out = list(splitter.feed(chunks))
        assert out == [("测试文本。", emo)], f"failed for {emo}"
