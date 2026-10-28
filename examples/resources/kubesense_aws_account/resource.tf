resource "kubesense_aws_account" "prod" {
  account_id = "123456789012"
  mode       = "role_access"
  role_arn   = "arn:aws:iam::123456789012:role/KubeSenseCollectorRole"

  resource_types = {
    ec2 = { collect = true, metric_enabled = true }
    rds = { collect = true, metric_enabled = true, metric_tier = "expanded" }
  }
}
