# PostgreSQL RDS 模块
variable "engine_version" { type = string, default = "16.3" }
variable "instance_class" { type = string, default = "db.r6g.large" }
variable "allocated_storage_gb" { type = number, default = 100 }

resource "aws_db_instance" "main" {
  engine                = "postgres"
  engine_version        = var.engine_version
  instance_class        = var.instance_class
  allocated_storage     = var.allocated_storage_gb
  storage_encrypted     = true
  db_name               = "aicity"
  username              = "aicity"
  password              = var.db_password
  skip_final_snapshot   = false
  backup_retention_period = 7
  multi_az              = true

  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]

  tags = {
    Project = "ai-city"
  }
}

output "endpoint" { value = aws_db_instance.main.endpoint }
