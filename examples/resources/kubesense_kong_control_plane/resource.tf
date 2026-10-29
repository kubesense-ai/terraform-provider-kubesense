# Kong Konnect: one integration per control plane.
resource "kubesense_kong_control_plane" "konnect" {
  org_id           = "acme"
  mode             = "konnect"
  base_url         = "https://us.api.konghq.com"
  control_plane_id = "6c1f0a3e-8d2b-4e4f-9a7c-1b3d5f7e9a21"
  bearer_token     = var.konnect_token
}

# A self-hosted gateway, reached through its Admin API.
resource "kubesense_kong_control_plane" "self_hosted" {
  org_id        = "acme-onprem"
  mode          = "self_hosted"
  base_url      = "https://kong-admin.example.com:8444"
  admin_api_key = var.kong_admin_api_key
}
