package cluster

import (
	"context"
	"fmt"
	"os"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// IssueProjectKubeconfig creates (or reuses) a ServiceAccount in the given
// project namespace, ensures it has namespaced RBAC consistent with the
// requested role, and returns a kubeconfig that targets the Capsule proxy
// using a bound service-account token. The kubeconfig never points at the
// raw cluster API server.
func IssueProjectKubeconfig(
	ctx context.Context,
	clusterKubeconfig []byte,
	proxyURL string,
	proxyCA string,
	tenant string,
	project string,
	role string,
	ttlSeconds int,
) ([]byte, time.Time, error) {
	if tenant == "" || project == "" {
		return nil, time.Time{}, fmt.Errorf("tenant and project required")
	}
	nsName := AppNamespaceName(tenant, project)
	if err := EnsureProjectNamespace(ctx, clusterKubeconfig, tenant, project); err != nil {
		return nil, time.Time{}, err
	}
	if role == "" {
		role = "readOnly"
	}
	return issueNamespaceKubeconfig(ctx, clusterKubeconfig, proxyURL, proxyCA, tenant, nsName, project, role, ttlSeconds)
}

func IssueSandboxKubeconfig(
	ctx context.Context,
	clusterKubeconfig []byte,
	proxyURL string,
	proxyCA string,
	tenant string,
	sandbox string,
	ttlSeconds int,
) ([]byte, time.Time, error) {
	if tenant == "" || sandbox == "" {
		return nil, time.Time{}, fmt.Errorf("tenant and sandbox required")
	}
	nsName := SandboxNamespaceName(tenant, sandbox)
	if err := EnsureSandboxNamespace(ctx, clusterKubeconfig, tenant, sandbox); err != nil {
		return nil, time.Time{}, err
	}
	return issueNamespaceKubeconfig(ctx, clusterKubeconfig, proxyURL, proxyCA, tenant, nsName, sandbox, "tenantOwner", ttlSeconds)
}

func issueNamespaceKubeconfig(
	ctx context.Context,
	clusterKubeconfig []byte,
	proxyURL string,
	proxyCA string,
	tenant string,
	namespace string,
	project string,
	role string,
	ttlSeconds int,
) ([]byte, time.Time, error) {
	if tenant == "" || project == "" {
		return nil, time.Time{}, fmt.Errorf("tenant and project required")
	}
	if namespace == "" {
		return nil, time.Time{}, fmt.Errorf("namespace required")
	}
	if proxyURL == "" {
		return nil, time.Time{}, fmt.Errorf("proxy url required")
	}
	if parseBool(os.Getenv("KUBENOVA_E2E_FAKE")) {
		if ttlSeconds <= 0 {
			ttlSeconds = 3600
		}
		exp := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)
		cfgBytes := buildProxyKubeconfig(proxyURL, namespace, "dev-token", proxyCA)
		return cfgBytes, exp, nil
	}
	if role == "" {
		role = "readOnly"
	}
	switch role {
	case "tenantOwner", "projectDev", "readOnly":
	default:
		return nil, time.Time{}, fmt.Errorf("unsupported role %q", role)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(clusterKubeconfig)
	if err != nil {
		return nil, time.Time{}, err
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, time.Time{}, err
	}

	saName := projectRoleName(tenant, project, role)
	if err := ensureServiceAccount(ctx, cset, namespace, saName); err != nil {
		return nil, time.Time{}, err
	}
	if err := ensureNamespacedRBAC(ctx, cset, namespace, saName, tenant, project, role); err != nil {
		return nil, time.Time{}, err
	}
	if role == "tenantOwner" {
		if err := ensureTenantNamespacesView(ctx, cset, namespace, saName, tenant); err != nil {
			return nil, time.Time{}, err
		}
	}

	token, exp, err := issueServiceAccountToken(ctx, cset, namespace, saName, ttlSeconds)
	if err != nil {
		return nil, time.Time{}, err
	}

	cfgBytes := buildProxyKubeconfig(proxyURL, namespace, token, proxyCA)
	return cfgBytes, exp, nil
}

func ensureServiceAccount(ctx context.Context, cset kubernetes.Interface, namespace, name string) error {
	saClient := cset.CoreV1().ServiceAccounts(namespace)
	if _, err := saClient.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		_, err = saClient.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func ensureNamespacedRBAC(ctx context.Context, cset kubernetes.Interface, namespace, saName, tenant, project, role string) error {
	roleClient := cset.RbacV1().Roles(namespace)
	rbClient := cset.RbacV1().RoleBindings(namespace)

	roleName := projectRoleName(tenant, project, role)
	rules := roleRulesFor(role)
	desiredRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: namespace},
		Rules:      rules,
	}
	if existing, err := roleClient.Get(ctx, roleName, metav1.GetOptions{}); err == nil {
		// Update rules to stay in sync with helpers.
		existing.Rules = rules
		if _, err := roleClient.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return err
		}
	} else {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if _, err := roleClient.Create(ctx, desiredRole, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: namespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Namespace: namespace,
			Name:      saName,
		}},
	}
	if existing, err := rbClient.Get(ctx, roleName, metav1.GetOptions{}); err == nil {
		existing.Subjects = rb.Subjects
		if _, err := rbClient.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return err
		}
	} else {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if _, err := rbClient.Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func issueServiceAccountToken(ctx context.Context, cset kubernetes.Interface, namespace, saName string, ttlSeconds int) (string, time.Time, error) {
	saClient := cset.CoreV1().ServiceAccounts(namespace)

	// TTL bounds and default, aligned with HTTP API semantics.
	if ttlSeconds < 0 {
		ttlSeconds = 0
	}
	if ttlSeconds > 315360000 {
		ttlSeconds = 315360000
	}

	tr := &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{}}
	if ttlSeconds > 0 {
		sec := int64(ttlSeconds)
		tr.Spec.ExpirationSeconds = &sec
	}

	res, err := saClient.CreateToken(ctx, saName, tr, metav1.CreateOptions{})
	if err != nil {
		return "", time.Time{}, err
	}
	tok := res.Status.Token
	exp := res.Status.ExpirationTimestamp.Time
	// When ExpirationTimestamp is zero and TTL was not requested, return a
	// zero time; callers may choose to omit expiresAt in responses.
	return tok, exp, nil
}

func buildProxyKubeconfig(server, namespace, token, caData string) []byte {
	if server == "" {
		server = "https://proxy.kubenova.svc"
	}
	nsLine := ""
	if namespace != "" {
		nsLine = "    namespace: " + namespace + "\n"
	}
	clusterBlock := ""
	if caData != "" {
		clusterBlock = "    certificate-authority-data: " + caData + "\n    server: " + server + "\n"
	} else {
		clusterBlock = "    insecure-skip-tls-verify: true\n    server: " + server + "\n"
	}
	cfg := "apiVersion: v1\nkind: Config\nclusters:\n- name: kn-proxy\n  cluster:\n" + clusterBlock + "contexts:\n- name: tenant\n  context:\n    cluster: kn-proxy\n    user: tenant-user\n" + nsLine + "current-context: tenant\nusers:\n- name: tenant-user\n  user:\n    token: " + token + "\n"
	return []byte(cfg)
}

func ensureTenantNamespacesView(ctx context.Context, cset kubernetes.Interface, namespace, saName, tenant string) error {
	crClient := cset.RbacV1().ClusterRoles()
	crbClient := cset.RbacV1().ClusterRoleBindings()
	crName := projectRoleName(tenant, "all", "tenantowner-ns")
	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list"},
		},
	}
	if cr, err := crClient.Get(ctx, crName, metav1.GetOptions{}); err == nil {
		cr.Rules = rules
		if _, err := crClient.Update(ctx, cr, metav1.UpdateOptions{}); err != nil {
			return err
		}
	} else {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cr := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: crName},
			Rules:      rules,
		}
		if _, err := crClient.Create(ctx, cr, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	crbName := crName + "-binding"
	subjects := []rbacv1.Subject{{
		Kind:      "ServiceAccount",
		Namespace: namespace,
		Name:      saName,
	}}
	if crb, err := crbClient.Get(ctx, crbName, metav1.GetOptions{}); err == nil {
		crb.Subjects = subjects
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     crName,
		}
		if _, err := crbClient.Update(ctx, crb, metav1.UpdateOptions{}); err != nil {
			return err
		}
	} else {
		if !apierrors.IsNotFound(err) {
			return err
		}
		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: crbName},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     crName,
			},
			Subjects: subjects,
		}
		if _, err := crbClient.Create(ctx, crb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}
