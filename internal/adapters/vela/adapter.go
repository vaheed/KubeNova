package vela

import "github.com/vaheed/kubenova/pkg/types"

// AppAdapter translates apps to KubeVela-friendly structures.
type AppAdapter struct{}

// NewAppAdapter creates a new adapter.
func NewAppAdapter() *AppAdapter {
	return &AppAdapter{}
}

// ToApplication renders a simplified application payload.
func (a *AppAdapter) ToApplication(app *types.App) map[string]any {
	if app == nil {
		return map[string]any{}
	}
	return map[string]any{
		"name":        app.Name,
		"component":   app.Component,
		"image":       app.Image,
		"spec":        app.Spec,
		"traits":      app.Traits,
		"policies":    app.Policies,
		"revision":    app.Revision,
		"projectId":   app.ProjectID,
		"clusterId":   app.ClusterID,
		"environment": app.TenantID,
	}
}
