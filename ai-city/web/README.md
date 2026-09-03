# web (玩家端)

> **职责**：玩家端 Next.js 15 应用
>
> **技术栈**：Next.js 15 + React 19 + MapLibre GL JS + Zustand + TanStack Query
>
> **关键文档**：[docs/11-技术细节与玩法模式.md §E.6 §E.7](../../docs/11-技术细节与玩法模式.md)

## 路由

```
/                   - 落地页
/city               - 主城市视图（地图 + HUD + Chat）
/city/[tileId]      - 特定 Tile 视图
/chat               - 聊天界面（脱离地图）
/inventory          - 物品栏
/settings           - 设置
```

## 关键依赖

- `@aicity/client-reconciler` - 客户端预测 + 服务端协调（§E.6）
- `@aicity/offline-sync` - 离线日志同步（§E.7）
- `@aicity/bypass-filter` - 旁路流过滤（§E.4）
- `maplibre-gl` - 地图渲染

## 端口

`3000`
