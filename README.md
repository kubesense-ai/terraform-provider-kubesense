# terraform-provider-kubesense

The KubeSense Terraform provider. It manages dashboards, dashboard lists,
platform integrations (Jira, Confluence, Atlassian, custom MCP servers) and
cloud account integrations.

## Using it

```hcl
terraform {
  required_providers {
    kubesense = {
      source  = "kubesense-ai/kubesense"
      version = "~> 1.0"
    }
  }
}

provider "kubesense" {
  api_url = "https://kubesense.example.com" # or KUBESENSE_API_URL
  # api_key from KUBESENSE_API_KEY
}
```

One provider works with every KubeSense installation: the installation is
chosen by `api_url`, not by the provider's address. Provider and server
versions move independently; see [Version compatibility](#version-compatibility).

This repository is generated from KubeSense's main repository, where the
provider is developed next to the API it talks to. `internal/kschema` is a copy
of the API's dashboard and alert contract. Issues are welcome here; changes are
made upstream and appear here with the next sync.

## Resources and data sources

| Name | Kind |
|---|---|
| `kubesense_dashboard` | resource: panels, variables and layout as typed blocks |
| `kubesense_dashboard_json` | resource: the whole preset as one JSON string |
| `kubesense_dashboard_list` | resource |
| `kubesense_alert_rule` | resource: an alert rule as typed blocks, channels by name |
| `kubesense_integration` | resource |
| `kubesense_aws_account` | resource |
| `kubesense_gcp_project` | resource |
| `kubesense_azure_subscription` | resource |
| `kubesense_mongodbatlas_org` | resource |
| `kubesense_confluent_org` | resource |
| `kubesense_kong_control_plane` | resource |
| `kubesense_dashboard` | data source |
| `kubesense_integration` | data source |

The six cloud resources share one implementation (`cloud_account_resource.go`).
Only the table in `cloud_specs.go` differs between them, so adding a cloud is a
new entry there.

Examples are under `examples/`.

## Authentication

The provider uses a service-account key, sent as `x-api-key`.

1. An admin creates a service account in **Settings → Service accounts**. Its
   role needs **full access** to Dashboards.
2. Create a key on that account with scopes `dashboard`, `integrations` and
   `alerts` at `write`, for what you manage.
3. Export the key and the deployment URL:

   ```sh
   export KUBESENSE_API_URL=https://kubesense.example.com
   export KUBESENSE_API_KEY=<key>
   ```

## Developing

Build and test:

```sh
go build ./...
go test ./...                  # unit tests
TF_ACC=1 go test ./...         # plus acceptance tests, which need terraform on PATH
```

The acceptance tests run real plan, apply and import cycles against an
in-memory fake of the API (`internal/provider/acceptance_test.go`), so they need
no running kubeapi.

### Using a local build with Terraform

1. Install the binary:

   ```sh
   go install .
   ```

2. Point Terraform at it with a development override in `~/.terraformrc`:

   ```hcl
   provider_installation {
     dev_overrides {
       "kubesense-ai/kubesense" = "/Users/<you>/go/bin"
     }
     direct {}
   }
   ```

3. Skip `terraform init` and run `terraform plan` / `terraform apply` directly.
   Terraform prints a warning that the override is active.


### Debugging

Run the provider under a debugger with `-debug`. It prints a `TF_REATTACH_PROVIDERS`
value to export in the shell where you run terraform.

## Behaviour worth knowing

- **Typed dashboards share their conversion with kubeapi.** The schema and both
  conversions come from kubeapi's `schema/terraform` package, which derives the
  Terraform shape from the dashboard JSON Schema. The webapp's Export to
  Terraform (`POST /api/dashboard/terraform`) uses the same package, so an
  exported dashboard imports with no changes.
  - Members Terraform cannot type are `*_json` attributes, set with
    `jsonencode()`.
  - Unknown members ride in `extra_json`.
- **Alert rules are their definitions.** `kubesense_alert_rule` is the stored
  rule as Export writes it, without server state, and with notification
  channels by name (`notification_channel_names`), since names are the same in
  every install and ids are not. kubeapi converts definitions to and from rules
  (`/alerts/rules/definition`), so a Terraform write goes through the same
  validation as the editor's, and the contract is enforced strictly.
  - The server fills defaults a hand-written rule may leave out (`enabled`,
    `sample_limit`, a query's `visible`). The provider reads back only what the
    configuration sets, so those never show as changes; a change to anything
    the configuration sets does. An import adopts everything.
- **`dashboard_json` comparison** (kubesense_dashboard_json). Changes that only affect formatting, key
  order or the server's schema migration never show up in a plan. The provider
  compares migrated, normalised JSON, and sends presets already migrated and
  with a `tabs` key.
- **Strict validation.** kubeapi recognises the provider's User-Agent and
  refuses every dashboard contract finding. The UI's own path only logs them.
  The same check runs at plan time, against the server's contract
  (`POST /dashboard/validate`), so errors show up before apply. A server too
  old for that endpoint is checked against the contract built into the
  provider. `terraform validate` has no server to ask, so it only checks that
  `dashboard_json` is a JSON object.
- **Provider and server versions move independently.** See
  [Version compatibility](#version-compatibility).
- **System dashboards** are read-only. Read one with the data source, and build
  your own dashboard from its `dashboard_json`.
- **Integrations test the connection.** Creating an integration, or changing its
  credentials or config, makes KubeSense connect to the target first and refuse
  the change if that fails. `terraform apply` therefore needs a deployment that
  can reach Jira, Confluence, the MCP server or the cloud, with working
  credentials.
- **Secrets are write-only.** `api_key` and the secret cloud credentials are
  never returned by the API. A change made outside Terraform is not detected,
  and they are not set on import.
- **Values the server normalises** (a trimmed URL, say) do not show as changes.
  The provider records what the server stored at each apply, in private state,
  and only reports a value that moved after that.
- **Removing a credential from the configuration keeps it stored.** The API
  merges credentials and has no way to delete one.
- **`resource_types` on cloud resources:**
  - when set, Terraform owns collection, and types enabled in the UI but not
    listed are switched off on the next apply;
  - when omitted, collection is left alone.
- **GitHub** connects only through the UI's app flow. Read it with the
  `kubesense_integration` data source.
- **`kubesense_dashboard_list.dashboard_ids`:**
  - when set, Terraform owns membership, and dashboards added in the UI are
    removed on the next apply;
  - when omitted, membership is left alone.

## Version compatibility

Users pin this provider and upgrade it on their own schedule, separately from
the KubeSense server it talks to. Either one can be the newer of the two, and
upgrading either must not change a plan that was empty before.

### What makes it hold

| Skew | Mechanism |
|---|---|
| Provider **older** than the server | Every request sends `KubeSense-Schema-Versions` (the schema versions this build knows). The server answers dashboard and alert-rule reads in those versions, converting down through each migration step's `Down`. kubeapi's `migrate.Check` refuses a chain with a step from v2 on that has no `Down`. |
| Provider **newer** than the server | The provider reads `GET /schema/versions` once per run and converts what it writes down to the server's version. A server without that endpoint is v1 throughout. |
| The server's contract has **widened** (a new panel type, a new option) | Plan-time checks ask the server (`POST /dashboard/validate`), so the provider never refuses something the server accepts. |
| The server has **no endpoint** for a resource (it is too old, or was rolled back) | The client recognises kubeapi's catch-all 404 (`"Endpoint doesn't exists"`) and fails with "upgrade KubeSense or pin the provider". It does **not** treat that as "deleted outside Terraform", so it never drops the resource from state and never plans to recreate it. |

### Rules for changing the provider

- **Never remove or rename an attribute, and never change its type, within a
  major version.** Add the new one beside it, and deprecate the old one. If an
  attribute has to change, the resource needs a `SchemaVersion` bump and an
  `UpgradeState` for state written by the earlier provider.
- **Never send a value the user did not set.** A new optional attribute stays
  null and stays out of the request, so an older server receives exactly what it
  received before.
- **Never check the contract more strictly than the server does.** Contract
  findings come from the server at plan time; the embedded contract is only the
  fallback for servers without `/dashboard/validate`.
- **A new resource on a new endpoint needs nothing extra.** On an older server
  its first request fails with the missing-endpoint message, which is the right
  answer.

### Tests that hold it

`internal/provider/acceptance_skew_test.go` runs each case through the real
`terraform` binary against the in-memory API, which can be switched between an
old server and a current one:

- server upgraded under an existing configuration → the plan is empty;
- server contract wider than the provider's → applies;
- server refuses → the plan fails with the server's finding and writes nothing;
- server too old for the dry run → the embedded contract is used;
- server missing a resource's endpoint → the plan fails and the resource stays
  in state.

`TestAccProviderUpgradeLeavesThePlanEmpty` (`acceptance_upgrade_test.go`) covers
the remaining case: state written by the **previous published release**, then
planned with this build, must give an empty plan. It downloads that release from
the registry, so it runs only when `KUBESENSE_PREVIOUS_PROVIDER_VERSION` is set.
The release job sets it to the latest published version.
