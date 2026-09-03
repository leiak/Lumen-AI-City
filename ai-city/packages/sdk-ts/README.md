# @aicity/sdk (TypeScript)

> **职责**：第三方 Agent 接入 AI City 联邦的 TypeScript SDK
>
> **关键文档**：[docs/06-A2A协议.md §20.17](../../docs/06-A2A协议.md)

## 安装

```bash
pnpm add @aicity/sdk
```

## 用法

```typescript
import { A2AClient } from '@aicity/sdk';

const client = new A2AClient(
  'https://aicity.example.com/a2a',
  'sk-xxx',
);

await client.registerCard({
  agent_id: 'my_agent_001',
  name: 'My Agent',
  url: 'https://my-agent.example.com',
  provider: 'openclaw',
  capabilities: ['dialogue', 'search'],
});

const peers = await client.discover('dialogue');
await client.sendMessage('npc_wang_boss_001', { hello: 'world' });
```
