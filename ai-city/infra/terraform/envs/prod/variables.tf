variable "db_password" {
  type      = string
  sensitive = true
  description = "PostgreSQL master password (从 Vault 注入)"
}
