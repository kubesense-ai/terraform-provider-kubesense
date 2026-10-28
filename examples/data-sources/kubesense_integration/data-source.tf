# GitHub is connected in the UI; read it to reference its id elsewhere.
data "kubesense_integration" "github" {
  integration_provider = "github"
  name                 = "acme"
}
