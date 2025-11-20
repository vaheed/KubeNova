package tenancy

import "strings"

// TenantDevGroup returns the delegated group name (tenant-devs) for the given tenant.
func TenantDevGroup(tenant string) string {
	name := sanitize(tenant)
	return name + "-devs"
}

func sanitize(input string) string {
	s := strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	b.Grow(len(s))
	prevHyphen := false
	for _, r := range s {
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
		out = "tenant"
	}
	if len(out) > 25 {
		out = out[:25]
		out = strings.Trim(out, "-")
		if out == "" {
			out = "tenant"
		}
	}
	return out
}
