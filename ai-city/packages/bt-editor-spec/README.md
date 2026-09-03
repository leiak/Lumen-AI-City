# packages/bt-editor-spec

> **职责**：行为树可视化编辑器的 JSON Schema 定义
>
> **关键文档**：[docs/12-BT编辑器PRD.md](../../docs/12-BT编辑器PRD.md) §5.1 / §9

## 文件清单

- `bt-node.schema.json` - 7 类节点定义（Sequence / Selector / Condition / Action / Decorator / SubTree / LLM）
- `bt-tree.schema.json` - 完整树结构

## 使用

```typescript
import btNodeSchema from '@aicity/bt-editor-spec/bt-node.schema.json';
import Ajv from 'ajv';

const ajv = new Ajv();
const validate = ajv.compile(btNodeSchema);
if (!validate(btJson)) {
  throw new Error('BT validation failed: ' + JSON.stringify(validate.errors));
}
```
