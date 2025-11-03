package vela

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// ApplicationCR builds a minimal KubeVela Application in unstructured form.
// apiVersion: core.oam.dev/v1beta1, kind: Application
func ApplicationCR(ns, name, image string) *unstructured.Unstructured {
	if ns == "" {
		ns = "default"
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": name, "namespace": ns},
		"spec": map[string]interface{}{
			"components": []interface{}{
				map[string]interface{}{
					"name":       name,
					"type":       "webservice",
					"properties": map[string]interface{}{"image": image},
				},
			},
		},
	}}
	return u
}
