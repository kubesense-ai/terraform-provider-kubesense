# System dashboards are read-only; copy one into a dashboard of your own.
data "kubesense_dashboard" "aws_ec2" {
  name = "AWS EC2 overview"
}

resource "kubesense_dashboard_json" "my_ec2" {
  name           = "EC2 – team view"
  dashboard_json = data.kubesense_dashboard.aws_ec2.dashboard_json
}
