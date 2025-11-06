package httperr

import (
	"encoding/json"
	"net/http"
)

// Write writes a KubeNova error payload with a KN-xxx code and message.
func Write(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}
