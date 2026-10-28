# Panels, variables and layout as blocks. Export to Terraform in the KubeSense
# UI writes this for an existing dashboard; members Terraform cannot type are
# *_json attributes set with jsonencode().
resource "kubesense_dashboard" "checkout" {
  name        = "Checkout"
  description = "Golden signals for checkout"

  grid_layout {
    h = 8
    i = "p1"
    w = 12
    x = 0
    y = 0
  }

  panel {
    name       = "CPU by pod"
    panel_type = "timeSeries"

    query {
      label         = "A"
      promql        = "sum(rate(container_cpu_usage_seconds_total[5m])) by (pod)"
      query_mode    = "code"
      selected_mode = "metrics"
    }
  }

  variable {
    description = "Cluster to show"
    name        = "cluster"

    meta {
      default_value = "prod"
      value_json    = jsonencode("prod")
      variable_type = "textbox"
    }
  }
}
