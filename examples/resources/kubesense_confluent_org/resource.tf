resource "kubesense_confluent_org" "prod" {
  org_id     = "b0b21724-4586-4a07-b787-d0bb5aacbf87"
  api_key    = var.confluent_cloud_api_key
  api_secret = var.confluent_cloud_api_secret
}
