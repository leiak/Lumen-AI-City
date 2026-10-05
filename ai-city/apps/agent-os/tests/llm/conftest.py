"""llm/ 测试 fixture + --run-real-llm 选项。"""


def pytest_addoption(parser):
    parser.addoption(
        "--run-real-llm",
        action="store_true",
        default=False,
        help="运行真 LLM API 集成测试（需要 API key 和网络）",
    )
