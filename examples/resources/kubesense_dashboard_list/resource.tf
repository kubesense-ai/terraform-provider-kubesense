resource "kubesense_dashboard_list" "payments" {
  name        = "Payments"
  description = "Owned by the payments team"
  dashboard_ids = [
    kubesense_dashboard_json.checkout.id,
  ]
}
