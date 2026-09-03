# infra/terraform - 基础设施即代码

## 结构

```
terraform/
├── modules/          # 可复用模块
│   ├── k8s/
│   ├── postgres/
│   ├── redis/
│   ├── kafka/
│   ├── neo4j/
│   ├── milvus/
│   └── grafana/
└── envs/             # 环境
    ├── dev/
    ├── staging/
    └── prod/
```

## 用法

```bash
cd envs/prod
terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

## 关键决策

- **跨云一致性**：所有模块在 AWS / 阿里云 / GCP 通用
- **状态**：远端 S3 / 阿里 OSS（带 state lock）
- **Secrets**：不放在 Terraform 中，统一用 Vault / Sealed Secrets
