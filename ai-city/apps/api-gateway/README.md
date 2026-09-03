# api-gateway

> **职责**：路由 / 限流 / 鉴权 / 反爬 / 配额 / Trace 注入
>
> **语言**：Go 1.23 + Gin
>
> **关键文档**：[docs/04-API设计.md §18.7](../../docs/04-API设计.md) / [docs/09-架构优化v2.md §34](../../docs/09-架构优化v2.md)

## 路由

```
GET  /health
POST /v1/auth/login
GET  /v1/players/:id
GET  /v1/npcs/:id
POST /v1/npcs/:id/dialogue
POST /v1/sagas
GET  /v1/sagas/:id
```

## 中间件链（顺序关键）

```
Recovery -> TraceID -> Logging -> RateLimit -> Auth -> AntiScrap -> Handler
```

## 本地启动

```bash
go run ./cmd
```

## 端口

`8080`
