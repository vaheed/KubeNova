package catalog

import "embed"

// FS exposes the embedded catalog data (plans, policysets) used by the API.
// The root of this filesystem is the pkg/catalog directory.
//
//go:embed plans.json policysets.json
var FS embed.FS
