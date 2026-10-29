# Organization-level access: an organization API key sees every project.
resource "kubesense_mongodbatlas_org" "prod" {
  org_id      = "5f3e8a1b2c4d6e7f8a9b0c1d"
  public_key  = "abcdefgh"
  private_key = var.atlas_org_private_key
}
