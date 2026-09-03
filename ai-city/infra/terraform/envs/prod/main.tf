# Production 环境
provider "aws" {
  region = "ap-northeast-1"  # 东京
}

module "k8s" {
  source          = "../../modules/k8s"
  cluster_name    = "aicity-prod"
  region          = "ap-northeast-1"
  node_count      = 10
  subnet_ids      = ["subnet-xxx", "subnet-yyy"]
}

module "postgres" {
  source               = "../../modules/postgres"
  instance_class       = "db.r6g.2xlarge"
  allocated_storage_gb = 500
  db_password          = var.db_password
}

module "redis" {
  source = "../../modules/redis"
  node_type = "cache.r6g.large"
}

variable "db_password" {
  type      = string
  sensitive = true
}

output "cluster_endpoint" { value = module.k8s.cluster_endpoint }
output "db_endpoint" { value = module.postgres.endpoint }
