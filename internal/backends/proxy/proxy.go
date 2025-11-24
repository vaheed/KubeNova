package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface abstracts publishing endpoints to the access proxy.
type Interface interface {
	Publish(ctx context.Context, tenant string, endpoint string) error
}

type clientImpl struct {
	client     client.Client
	apiBase    string
	httpClient *http.Client
}

// NewClient returns a proxy publisher that prefers the Capsule Proxy API when configured via
// PROXY_API_URL. If not configured, it falls back to the legacy ConfigMap traceability approach.
func NewClient(c client.Client) Interface {
	return &clientImpl{
		client:     c,
		apiBase:    os.Getenv("PROXY_API_URL"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *clientImpl) Publish(ctx context.Context, tenant string, endpoint string) error {
	if tenant == "" {
		return nil
	}
	if c.apiBase != "" {
		return c.publishAPI(ctx, tenant, endpoint)
	}
	return c.publishConfigMap(ctx, tenant, endpoint)
}

func (c *clientImpl) publishAPI(ctx context.Context, tenant, endpoint string) error {
	payload := map[string]string{"tenant": tenant, "endpoint": endpoint}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/v1/tenants/%s", c.apiBase, tenant)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("proxy publish failed: %s", resp.Status)
	}
	return nil
}

func (c *clientImpl) publishConfigMap(ctx context.Context, tenant, endpoint string) error {
	name := "kubenova-proxy-" + tenant
	ns := "kube-system"
	var cm corev1.ConfigMap
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &cm)
	if apierrors.IsNotFound(err) {
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    map[string]string{"managed-by": "kubenova"},
			},
			Data: map[string]string{
				"endpoint": endpoint,
			},
		}
		return c.client.Create(ctx, &cm)
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["endpoint"] = endpoint
	return c.client.Update(ctx, &cm)
}
