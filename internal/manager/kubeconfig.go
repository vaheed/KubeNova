package manager

// GenerateKubeconfig builds a minimal kubeconfig binding to the provided proxy server URL.
// It does not issue real credentials; callers are expected to provide short-lived tokens via API.
func GenerateKubeconfig(_ interface{}, server string) ([]byte, error) {
    if server == "" { server = "https://proxy.kubenova.svc" }
    token := "placeholder"
    b := []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: kn-proxy\n  cluster:\n    insecure-skip-tls-verify: true\n    server: " + server + "\ncontexts:\n- name: tenant\n  context:\n    cluster: kn-proxy\n    user: tenant-user\ncurrent-context: tenant\nusers:\n- name: tenant-user\n  user:\n    token: " + token + "\n")
    return b, nil
}

