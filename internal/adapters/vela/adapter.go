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
	componentName := app.Component
	if componentName == "" {
		componentName = app.Name
	}
	var compType string
	var compProps map[string]any
	if app.Spec != nil {
		if t, ok := app.Spec["type"].(string); ok {
			compType = t
		}
		if p, ok := app.Spec["properties"].(map[string]any); ok {
			compProps = p
		}
	}
	component := map[string]any{
		"name": componentName,
		"type": compType,
	}
	if compProps != nil {
		component["properties"] = compProps
	}
	if len(app.Traits) > 0 {
		component["traits"] = app.Traits
	}
	manifest := map[string]any{
		"name":       app.Name,
		"components": []map[string]any{component},
	}
	if len(app.Policies) > 0 {
		manifest["policies"] = app.Policies
	}
	return manifest
}
