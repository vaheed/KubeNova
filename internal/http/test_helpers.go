package httpapi

import openapi_types "github.com/oapi-codegen/runtime/types"

const testClusterID = "3a7f5d62-2a0b-4b3e-bc39-3b3f1f33b111"

func uidStr(u *openapi_types.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}
