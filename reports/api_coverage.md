# API Coverage Summary

## ✅ Implemented and Tested (by example tests)
- POST /api/v1/clusters
- GET /api/v1/clusters/{c}
- GET /api/v1/clusters/{c}/capabilities
- POST /api/v1/clusters/{c}/tenants
- GET /api/v1/clusters/{c}/tenants/{t}
- PUT /api/v1/clusters/{c}/tenants/{t}/limits
- PUT /api/v1/clusters/{c}/tenants/{t}/network-policies
- POST /api/v1/clusters/{c}/tenants/{t}/projects
- PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps
- PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}
- GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/diff/{revA}/{revB}
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/image-update
- GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component}
- PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/policies
- GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/revisions
- GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status
- PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/traits
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:deploy
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:resume
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:rollback
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:suspend
- PUT /api/v1/clusters/{c}/tenants/{t}/quotas
- GET /api/v1/features
- GET /api/v1/me
- POST /api/v1/tokens
- GET /api/v1/version

## ⚠️ Missing or Partial (no direct tests found)
None — all previously flagged endpoints now have tests.

## 🚫 Unimplemented (missing handlers)
None — all OpenAPI routes have handlers.

# API Coverage Summary

| Endpoint | Tested | Test File |
|---|---|---|
| GET /api/v1/catalog/components | Y | internal/http/server_catalog_test.go |
| GET /api/v1/catalog/traits | Y | internal/http/server_catalog_test.go |
| GET /api/v1/catalog/workflows | Y | internal/http/server_catalog_test.go |
| POST /api/v1/clusters | Y | internal/manager/server_test.go |
| GET /api/v1/clusters/{c} | Y | internal/manager/server_test.go |
| POST /api/v1/clusters/{c}/bootstrap/{component} | Y | internal/http/server_additional_coverage_test.go |
| GET /api/v1/clusters/{c}/capabilities | Y | internal/http/server_test.go |
| GET /api/v1/clusters/{c}/policysets/catalog | Y | internal/http/server_additional_coverage_test.go |
| POST /api/v1/clusters/{c}/tenants | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t} | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/limits | Y | internal/http/server_policies_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/network-policies | Y | internal/http/server_policies_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/owners | Y | internal/http/server_additional_coverage_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/policysets | Y | internal/http/server_policysets_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/policysets/{name} | Y | internal/http/server_policysets_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p} | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access | Y | internal/http/server_additional_coverage_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/runs/{id} | Y | internal/http/server_policysets_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a} | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/diff/{revA}/{revB} | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/image-update | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component} | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/policies | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/revisions | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/traits | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/run | Y | internal/http/server_policysets_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/runs | Y | internal/http/server_policysets_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:delete | Y | internal/http/server_additional_coverage_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:deploy | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:resume | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:rollback | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:suspend | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig | Y | internal/http/server_policysets_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/quotas | Y | internal/http/server_policies_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/summary | Y | internal/http/server_additional_coverage_test.go |
| GET /api/v1/features | Y | internal/manager/server_system_test.go |
| GET /api/v1/healthz | Y | internal/http/server_health_ready_test.go |
| GET /api/v1/me | Y | internal/manager/server_tokens_test.go |
| GET /api/v1/projects/{p}/usage | Y | internal/http/server_additional_coverage_test.go |
| GET /api/v1/readyz | Y | internal/http/server_health_ready_test.go |
| POST /api/v1/tenants/{t}/kubeconfig | Y | internal/http/server_policysets_test.go |
| GET /api/v1/tenants/{t}/usage | Y | internal/http/server_additional_coverage_test.go |
| POST /api/v1/tokens | Y | internal/manager/server_tokens_test.go |
| GET /api/v1/version | Y | internal/manager/server_system_test.go |

## Missing/Unimplemented Methods
None.
