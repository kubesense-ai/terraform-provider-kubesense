terraform {
  required_providers {
    kubesense = {
      source = "kubesense-ai/kubesense"
    }
  }
}

# api_url and api_key can come from KUBESENSE_API_URL and KUBESENSE_API_KEY.
provider "kubesense" {
  api_url = "https://kubesense.example.com"
}
