resource "kubesense_gcp_project" "prod" {
  project_id               = "acme-prod-123"
  region                   = "us-central1"
  service_account_key_json = file("${path.module}/kubesense-sa.json")
}
