resource "kubesense_azure_subscription" "prod" {
  subscription_id = "00000000-0000-0000-0000-000000000000"
  tenant_id       = "11111111-1111-1111-1111-111111111111"
  client_id       = "22222222-2222-2222-2222-222222222222"
  client_secret   = var.azure_client_secret
  region          = "eastus"
}

variable "azure_client_secret" {
  type      = string
  sensitive = true
}
