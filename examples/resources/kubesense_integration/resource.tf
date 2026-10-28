resource "kubesense_integration" "jira" {
  integration_provider = "jira"
  name                 = "jira-prod"
  api_key              = var.jira_api_token
  config = {
    deployment = "cloud"
    base_url   = "https://acme.atlassian.net"
    username   = "kubesense-bot@acme.com"
  }
}

# A remote MCP server with its key in a custom header.
resource "kubesense_integration" "runbooks" {
  integration_provider = "custom"
  name                 = "runbooks-mcp"
  api_key              = var.runbooks_mcp_key
  auth_scheme          = "header"
  auth_header          = "X-API-Key"
  config = {
    deployment    = "remote"
    url           = "https://mcp.acme.internal/sse"
    extra_headers = jsonencode({ "X-Tenant" = "acme" })
  }
}

variable "jira_api_token" {
  type      = string
  sensitive = true
}

variable "runbooks_mcp_key" {
  type      = string
  sensitive = true
}
