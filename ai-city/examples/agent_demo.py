#!/usr/bin/env python3
"""Minimal external agent for AI City.

The demo registers through A2A, discovers an NPC, reads its shared talk tree,
and walks the tree through the gateway action API.

Run:
    python examples/agent_demo.py --once "有什么招牌菜？"

Optional natural-language option selection:
    $env:ARK_API_KEY="your-key"
    python examples/agent_demo.py
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any


DEFAULT_A2A = "http://localhost:8083"
DEFAULT_NPC = "npc_wang_boss_001"
DEFAULT_ARK_BASE = "https://ark.cn-beijing.volces.com/api/plan/v3"
DEFAULT_ARK_MODEL = "ark-code-latest"


@dataclass
class Option:
    id: str
    text: str


@dataclass
class NpcTurn:
    npc_id: str
    node_id: str
    say: str
    options: list[Option]


class HTTPError(RuntimeError):
    pass


class A2ANPCAgent:
    def __init__(
        self,
        base_url: str,
        agent_id: str,
        npc_id: str,
        ark_api_key: str = "",
        ark_base_url: str = DEFAULT_ARK_BASE,
        ark_model: str = DEFAULT_ARK_MODEL,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.agent_id = agent_id
        self.npc_id = npc_id
        self.ark_api_key = ark_api_key.strip()
        self.ark_base_url = ark_base_url.rstrip("/")
        self.ark_model = ark_model
        self.current: NpcTurn | None = None

    def _request(self, method: str, path: str, body: Any = None, headers: dict[str, str] | None = None) -> Any:
        url = path if path.startswith("http") else self.base_url + path
        data = None if body is None else json.dumps(body, ensure_ascii=False).encode("utf-8")
        request = urllib.request.Request(url, data=data, method=method)
        request.add_header("Content-Type", "application/json")
        request.add_header("Accept", "application/json")
        for key, value in (headers or {}).items():
            request.add_header(key, value)

        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                raw = response.read()
                return json.loads(raw.decode("utf-8")) if raw else {}
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode("utf-8", errors="replace")
            raise HTTPError(f"{method} {url} -> HTTP {exc.code}: {detail}") from exc
        except urllib.error.URLError as exc:
            raise HTTPError(f"{method} {url} -> {exc.reason}") from exc

    def register(self) -> None:
        response = self._request(
            "POST",
            "/v1/cards",
            {
                "agent_id": self.agent_id,
                "name": "NPC Dialogue Demo Agent",
                "description": "Example external agent that talks with city NPCs",
                "provider": "openclaw",
                "version": "1.0.0",
                "capabilities": ["dialogue"],
                "city_id": "city_a",
            },
        )
        if not response.get("accepted"):
            raise HTTPError(f"agent registration rejected: {response}")
        print(f"[agent] registered: {self.agent_id}")

    def discover_npcs(self) -> list[dict[str, Any]]:
        query = urllib.parse.urlencode({"capability": "dialogue", "city_filter": "city_a"})
        response = self._request("GET", f"/v1/discover?{query}")
        cards = [card for card in response.get("cards", []) if card.get("agent_id", "").startswith("npc_")]
        print("[agent] discovered NPCs:")
        for card in cards:
            print(f"  - {card.get('agent_id')} ({card.get('name') or 'unnamed'})")
        return cards

    def read_behavior_tree(self) -> dict[str, Any]:
        query = urllib.parse.urlencode({"npc_id": self.npc_id})
        return self._request("GET", f"/v1/agent/actions/npc-behavior?{query}")

    def start(self) -> NpcTurn:
        return self.talk("")

    def talk(self, node_id: str) -> NpcTurn:
        response = self._request(
            "POST",
            "/v1/agent/actions/npc-talk",
            {"agent_id": self.agent_id, "npc_id": self.npc_id, "node_id": node_id},
        )
        self.current = NpcTurn(
            npc_id=response["npc_id"],
            node_id=response["node_id"],
            say=response["say"],
            options=[Option(item["id"], item["text"]) for item in response.get("options", [])],
        )
        return self.current

    def choose_option(self, user_input: str) -> Option | None:
        if self.current is None:
            self.start()
        assert self.current is not None
        options = self.current.options
        if not options:
            return None

        text = user_input.strip()
        if text.isdigit():
            index = int(text) - 1
            if 0 <= index < len(options):
                return options[index]
            return None

        lowered = text.lower()
        for option in options:
            if text == option.id or lowered == option.text.lower():
                return option

        if self.ark_api_key:
            return self._choose_with_ark(options, text)
        return None

    def _choose_with_ark(self, options: list[Option], user_input: str) -> Option | None:
        assert self.current is not None
        choice_lines = "\n".join(f"{option.id}: {option.text}" for option in options)
        prompt = (
            "你是城市里的外部 Agent。根据用户输入，从 NPC 对话选项中选择最合适的一项。\n"
            f"NPC 刚才说：{self.current.say}\n"
            f"可用选项：\n{choice_lines}\n"
            "只回复选项 id，不要解释。"
        )
        payload = {
            "model": self.ark_model,
            "messages": [{"role": "user", "content": prompt}],
            "temperature": 0.1,
            "max_tokens": 512,
        }
        response = self._request(
            "POST",
            f"{self.ark_base_url}/chat/completions",
            payload,
            headers={"Authorization": f"Bearer {self.ark_api_key}"},
        )
        content = response["choices"][0]["message"]["content"].strip()
        for option in options:
            if option.id == content or option.text == content:
                return option
        print(f"[agent] LLM returned unknown option: {content!r}")
        return None


def print_turn(turn: NpcTurn) -> None:
    print(f"\n{turn.npc_id} [{turn.node_id}]>")
    print(turn.say)
    if turn.options:
        print("Options:")
        for index, option in enumerate(turn.options, start=1):
            print(f"  {index}. {option.id} - {option.text}")
    else:
        print("(conversation ended)")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Minimal AI City A2A NPC agent")
    parser.add_argument("--base-url", default=os.getenv("A2A_BASE_URL", DEFAULT_A2A))
    parser.add_argument("--agent-id", default=os.getenv("A2A_AGENT_ID", f"demo-agent-{int(time.time())}"))
    parser.add_argument("--npc-id", default=os.getenv("A2A_NPC_ID", DEFAULT_NPC))
    parser.add_argument("--once", metavar="INPUT", help="run one dialogue turn and exit")
    parser.add_argument("--show-tree", action="store_true", help="print the NPC talk tree and exit")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    agent = A2ANPCAgent(
        base_url=args.base_url,
        agent_id=args.agent_id,
        npc_id=args.npc_id,
        ark_api_key=os.getenv("ARK_API_KEY", ""),
        ark_base_url=os.getenv("ARK_BASE_URL", DEFAULT_ARK_BASE),
        ark_model=os.getenv("ARK_MODEL", DEFAULT_ARK_MODEL),
    )

    agent.register()
    agent.discover_npcs()

    if args.show_tree:
        tree = agent.read_behavior_tree()
        print(json.dumps(tree, ensure_ascii=False, indent=2))
        return 0

    print_turn(agent.start())

    if args.once is not None:
        option = agent.choose_option(args.once)
        if option is None:
            print(f"[agent] no matching option for: {args.once}")
            return 1
        print(f"[agent] chose option: {option.id} ({option.text})")
        print_turn(agent.talk(option.id))
        return 0

    print("\n输入编号、选项文字或自然语言；输入 q 退出。")
    while True:
        try:
            user_input = input("you> ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            return 0
        if user_input.lower() in {"q", "quit", "exit"}:
            return 0

        option = agent.choose_option(user_input)
        if option is None:
            if not agent.current or not agent.current.options:
                print("[agent] 当前节点没有下一项，输入 r 重新开始。")
            else:
                print("[agent] 没有匹配的选项。")
            continue
        if option.id == "leave":
            print_turn(agent.talk(option.id))
            print("[agent] 对话结束。")
            return 0
        print_turn(agent.talk(option.id))


if __name__ == "__main__":
    try:
        sys.exit(main())
    except HTTPError as error:
        print(f"[agent] ERROR: {error}", file=sys.stderr)
        sys.exit(2)
