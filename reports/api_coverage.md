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
- GET /api/v1/catalog/components
- GET /api/v1/catalog/traits
- GET /api/v1/catalog/workflows
- POST /api/v1/clusters/{c}/bootstrap/{component}
- GET /api/v1/clusters/{c}/policysets/catalog
- PUT /api/v1/clusters/{c}/tenants/{t}/owners
- POST /api/v1/clusters/{c}/tenants/{t}/policysets
- PUT /api/v1/clusters/{c}/tenants/{t}/policysets/{name}
- PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access
- GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/runs/{id}
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/run
- GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/runs
- POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:delete
- GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig
- GET /api/v1/clusters/{c}/tenants/{t}/summary
- GET /api/v1/healthz
- GET /api/v1/projects/{p}/usage
- GET /api/v1/readyz
- POST /api/v1/tenants/{t}/kubeconfig
- GET /api/v1/tenants/{t}/usage

## 🚫 Unimplemented (missing handlers)
- DeleteApiV1ClustersCTenantsTPolicysetsName
- GetApiV1CatalogComponents
- GetApiV1CatalogTraits
- GetApiV1CatalogWorkflows
- GetApiV1ClustersCPolicysetsCatalog
- GetApiV1ClustersCTenantsTPolicysets
- GetApiV1ClustersCTenantsTPolicysetsName
- GetApiV1ClustersCTenantsTProjectsPAppsAWorkflowRuns
- GetApiV1ClustersCTenantsTProjectsPAppsRunsId
- GetApiV1ClustersCTenantsTProjectsPKubeconfig
- GetApiV1Healthz
- GetApiV1ProjectsPUsage
- GetApiV1Readyz
- GetApiV1TenantsTUsage
- PostApiV1ClustersCBootstrapComponent
- PostApiV1ClustersCTenantsTPolicysets
- PostApiV1ClustersCTenantsTProjectsPAppsADelete
- PostApiV1ClustersCTenantsTProjectsPAppsAWorkflowRun
- PostApiV1TenantsTKubeconfig
- PutApiV1ClustersCTenantsTPolicysetsName

# API Coverage Summary

| Endpoint | Tested | Test File |
|---|---|---|
| GET /api/v1/catalog/components | N |  |
| GET /api/v1/catalog/traits | N |  |
| GET /api/v1/catalog/workflows | N |  |
| POST /api/v1/clusters | Y | internal/manager/server_test.go |
| GET /api/v1/clusters/{c} | Y | internal/manager/server_test.go |
| POST /api/v1/clusters/{c}/bootstrap/{component} | N |  |
| GET /api/v1/clusters/{c}/capabilities | Y | internal/http/server_test.go |
| GET /api/v1/clusters/{c}/policysets/catalog | N |  |
| POST /api/v1/clusters/{c}/tenants | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t} | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/limits | Y | internal/http/server_policies_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/network-policies | Y | internal/http/server_policies_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/owners | N |  |
| POST /api/v1/clusters/{c}/tenants/{t}/policysets | N |  |
| PUT /api/v1/clusters/{c}/tenants/{t}/policysets/{name} | N |  |
| POST /api/v1/clusters/{c}/tenants/{t}/projects | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p} | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access | N |  |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/runs/{id} | N |  |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a} | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/diff/{revA}/{revB} | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/image-update | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component} | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/policies | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/revisions | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status | Y | internal/http/server_apps_ops_test.go |
| PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/traits | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/run | N |  |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/runs | N |  |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:delete | N |  |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:deploy | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:resume | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:rollback | Y | internal/http/server_apps_ops_test.go |
| POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:suspend | Y | internal/http/server_apps_ops_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig | N |  |
| PUT /api/v1/clusters/{c}/tenants/{t}/quotas | Y | internal/http/server_policies_test.go |
| GET /api/v1/clusters/{c}/tenants/{t}/summary | N |  |
| GET /api/v1/features | Y | internal/manager/server_system_test.go |
| GET /api/v1/healthz | N |  |
| GET /api/v1/me | Y | internal/manager/server_tokens_test.go |
| GET /api/v1/projects/{p}/usage | N |  |
| GET /api/v1/readyz | N |  |
| POST /api/v1/tenants/{t}/kubeconfig | N |  |
| GET /api/v1/tenants/{t}/usage | N |  |
| POST /api/v1/tokens | Y | internal/manager/server_tokens_test.go |
| GET /api/v1/version | Y | internal/manager/server_system_test.go |

## Missing/Unimplemented Methods
- DeleteApiV1ClustersCTenantsTPolicysetsName
- GetApiV1CatalogComponents
- GetApiV1CatalogTraits
- GetApiV1CatalogWorkflows
- GetApiV1ClustersCPolicysetsCatalog
- GetApiV1ClustersCTenantsTPolicysets
- GetApiV1ClustersCTenantsTPolicysetsName
- GetApiV1ClustersCTenantsTProjectsPAppsAWorkflowRuns
- GetApiV1ClustersCTenantsTProjectsPAppsRunsId
- GetApiV1ClustersCTenantsTProjectsPKubeconfig
- GetApiV1Healthz
- GetApiV1ProjectsPUsage
- GetApiV1Readyz
- GetApiV1TenantsTUsage
- PostApiV1ClustersCBootstrapComponent
- PostApiV1ClustersCTenantsTPolicysets
- PostApiV1ClustersCTenantsTProjectsPAppsADelete
- PostApiV1ClustersCTenantsTProjectsPAppsAWorkflowRun
- PostApiV1TenantsTKubeconfig
- PutApiV1ClustersCTenantsTPolicysetsName
