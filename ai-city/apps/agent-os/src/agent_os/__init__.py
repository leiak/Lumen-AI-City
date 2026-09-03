"""Agent OS - AI Agent 五模块循环运行时。

详细设计见 docs/05-Agent-OS.md 全部内容，以及 docs/11-技术细节与玩法模式.md §B.1。
"""

from agent_os.loop import AgentRuntime
from agent_os.config import Settings

__version__ = "0.1.0"
__all__ = ["AgentRuntime", "Settings"]
