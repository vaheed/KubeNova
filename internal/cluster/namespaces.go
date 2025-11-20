package cluster

import (
	"strings"
)

const (
	appNamespacePrefix     = "tn-"
	appNamespaceSeparator  = "-app-"
	sandboxNamespaceSuffix = "-sandbox-"

	appTenantMaxLen    = 25
	appProjectMaxLen   = 25
	sandboxTenantMax   = 20
	sandboxNameMax     = 25
	namespaceMaxLength = 63

	LabelTenant        = "kubenova.tenant"
	LabelProject       = "kubenova.project"
	LabelNamespaceType = "kubenova.namespace-type"
	LabelSandbox       = "kubenova.io/sandbox"

	NamespaceTypeApp     = "app"
	NamespaceTypeSandbox = "sandbox"
)

// AppNamespaceName builds the namespace for a tenant's app project using the
// tn-<tenant>-app-<project> convention.
func AppNamespaceName(tenant, project string) string {
	return joinNamespace(appNamespacePrefix, appNamespaceSeparator, tenant, project, "app", appTenantMaxLen, appProjectMaxLen)
}

// SandboxNamespaceName builds the namespace for a tenant sandbox using the
// tn-<tenant>-sandbox-<name> convention.
func SandboxNamespaceName(tenant, sandbox string) string {
	return joinNamespace(appNamespacePrefix, sandboxNamespaceSuffix, tenant, sandbox, "sandbox", sandboxTenantMax, sandboxNameMax)
}

func joinNamespace(prefix, separator, tenant, part, kind string, tenantMax, partMax int) string {
	t := sanitizeSegment(tenant, "tenant", tenantMax)
	p := sanitizeSegment(part, kind, partMax)
	ns := prefix + t + separator + p
	if len(ns) > namespaceMaxLength {
		// Fallback: truncate the part segment to fit within the limit.
		allowed := namespaceMaxLength - len(prefix) - len(separator) - len(t)
		if allowed < 1 {
			allowed = 1
		}
		if allowed > len(p) {
			allowed = len(p)
		}
		p = p[:allowed]
		p = strings.Trim(p, "-")
		if p == "" {
			p = kind
		}
		ns = prefix + t + separator + p
	}
	return ns
}

func sanitizeSegment(value, fallback string, maxLen int) string {
	in := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(in))
	prevHyphen := false
	for _, r := range in {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else {
			if prevHyphen {
				continue
			}
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = fallback
	}
	if len(out) > maxLen {
		out = out[:maxLen]
		out = strings.Trim(out, "-")
		if out == "" {
			out = fallback
		}
	}
	return out
}

// ParseAppNamespace attempts to extract the tenant and project segments from an
// app namespace that follows the tn-<tenant>-app-<project> form.
func ParseAppNamespace(ns string) (tenant, project string, ok bool) {
	if !strings.HasPrefix(ns, appNamespacePrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(ns, appNamespacePrefix)
	parts := strings.SplitN(rest, appNamespaceSeparator, 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ParseSandboxNamespace extracts tenant and sandbox segments from a sandbox
// namespace that follows the tn-<tenant>-sandbox-<name> form.
func ParseSandboxNamespace(ns string) (tenant, sandbox string, ok bool) {
	if !strings.HasPrefix(ns, appNamespacePrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(ns, appNamespacePrefix)
	parts := strings.SplitN(rest, sandboxNamespaceSuffix, 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// IsSandboxNamespace reports whether the given namespace uses the sandbox pattern.
func IsSandboxNamespace(ns string) bool {
	_, _, ok := ParseSandboxNamespace(ns)
	return ok
}
