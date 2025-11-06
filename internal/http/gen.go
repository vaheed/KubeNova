package httpapi

//go:generate sh -c "oapi-codegen -generate types -package httpapi -o ./knapi_types.gen.go ../../docs/openapi/openapi.yaml"
//go:generate sh -c "oapi-codegen -generate chi-server -package httpapi -o ./knapi_server.gen.go ../../docs/openapi/openapi.yaml"
