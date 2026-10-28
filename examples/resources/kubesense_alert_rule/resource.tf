# Export to Terraform on an alert rule writes this for an existing rule.
resource "kubesense_alert_rule" "cpu_high" {
  name                       = "cpu high"
  severity                   = "critical"
  metric_query_label         = "A"
  threshold_operator         = "greater_than"
  threshold_value            = 80
  time_window                = "5m"
  evaluation_interval        = "1m"
  notification_channel_names = ["slack-oncall"]

  query_config {
    label         = "A"
    query_mode    = "code"
    selected_mode = "metrics"
    promql        = "avg(rate(container_cpu_usage_seconds_total[5m]))"
  }
}
