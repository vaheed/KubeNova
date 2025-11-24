package cluster

import (
	"context"
	"fmt"
)

// Installer coordinates bootstrap of in-cluster components.
// It is intentionally small in v0.0.1 and acts as a stub until the
// full Helm/manifest flow is wired.
type Installer struct{}

// NewInstaller returns a new Installer instance.
func NewInstaller() *Installer {
	return &Installer{}
}

// Bootstrap simulates a bootstrap action for the given component.
func (i *Installer) Bootstrap(ctx context.Context, component string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// RenderManifest returns a placeholder manifest for the requested component.
func (i *Installer) RenderManifest(component string) string {
	return fmt.Sprintf("# kubenova %s manifest would be rendered here", component)
}
