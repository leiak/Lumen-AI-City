# @aicity/bypass-filter

> **职责**：旁路流过滤（隐私保护）
>
> **关键文档**：[docs/11-技术细节与玩法模式.md §E.4](../../docs/11-技术细节与玩法模式.md)

## 用法

```typescript
import { bypassFilter } from '@aicity/bypass-filter';

const text = '我的手机号是 13812345678，邮箱是 user@example.com';
const { clean, redacted } = bypassFilter(text);

console.log(clean);     // 我的手机号是 1XX-XXXX-XXXX，邮箱是 X@X.com
console.log(redacted);  // { phone_cn: 1, email: 1 }
```

## 应用位置

- Agent OS → LLM 调用前
- Notification Engine → Push 文案生成
- Saga DSL → Safe-LLM 集成
