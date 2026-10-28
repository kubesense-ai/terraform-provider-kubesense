# A preset kept next to the configuration, e.g. from the dashboard's
# "Export to Terraform" action in the UI.
resource "kubesense_dashboard_json" "checkout" {
  name           = "Checkout service"
  description    = "Golden signals for checkout"
  dashboard_json = file("${path.module}/dashboards/checkout.json")
}

# A file from the dashboard's "Export" action wraps the preset in an envelope
# with the preset as a string; unwrap it like this.
resource "kubesense_dashboard_json" "from_export" {
  name           = "Payments"
  dashboard_json = jsondecode(file("${path.module}/payments.export.json")).preset
}

# One dashboard per JSON file in a folder.
resource "kubesense_dashboard_json" "service" {
  for_each       = fileset("${path.module}/dashboards", "*.json")
  name           = title(replace(trimsuffix(each.value, ".json"), "-", " "))
  dashboard_json = file("${path.module}/dashboards/${each.value}")
}
