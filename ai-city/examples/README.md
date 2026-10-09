# Agent Examples

## Minimal NPC dialogue agent

`agent_demo.py` is a zero-dependency Python example. It:

1. registers an external agent through A2A;
2. discovers dialogue-capable NPCs;
3. reads the NPC's shared behavior tree;
4. starts from the initial node and lets you select the next option;
5. optionally calls Ark to map natural-language input to an option.

### Run one deterministic turn

```bash
cd ai-city
python examples/agent_demo.py --once "有什么招牌菜？"
```

### Run interactively

```bash
cd ai-city
python examples/agent_demo.py
```

### Use Ark for natural-language choice

Do not put the API key in source control.

PowerShell:

```powershell
$env:ARK_API_KEY="your-ark-api-key"
$env:ARK_MODEL="ark-code-latest"
python examples/agent_demo.py
```

Bash:

```bash
export ARK_API_KEY="your-ark-api-key"
export ARK_MODEL="ark-code-latest"
python examples/agent_demo.py
```

Use `--npc-id` to talk with another discovered NPC, for example:

```bash
python examples/agent_demo.py --once "有没有什么冷门好书推荐？" --npc-id npc_book_keeper_001
```
