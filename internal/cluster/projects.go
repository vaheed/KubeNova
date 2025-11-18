package cluster

import (
	"context"
	"fmt"

	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
)

// EnsureProjectNamespace ensures a Namespace exists for the given tenant/project
// and is labeled for Capsule and KubeNova.
// Best-effort: if kubeconfig cannot be parsed, this becomes a no-op.
func EnsureProjectNamespace(ctx context.Context, kubeconfig []byte, tenant, project string) error {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		// In tests/dev the kubeconfig may be a stub; skip namespace creation.
		return nil
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	nsClient := cset.CoreV1().Namespaces()
	ns, err := nsClient.Get(ctx, project, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: project,
				Labels: map[string]string{
					"kubenova.project":          project,
					"kubenova.tenant":           tenant,
					"capsule.clastix.io/tenant": tenant,
				},
			},
		}
		_, err = nsClient.Create(ctx, ns, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}
	changed := false
	if ns.Labels["kubenova.project"] != project {
		ns.Labels["kubenova.project"] = project
		changed = true
	}
	if ns.Labels["kubenova.tenant"] != tenant {
		ns.Labels["kubenova.tenant"] = tenant
		changed = true
	}
	if ns.Labels["capsule.clastix.io/tenant"] != tenant {
		ns.Labels["capsule.clastix.io/tenant"] = tenant
		changed = true
	}
	if changed {
		_, err = nsClient.Update(ctx, ns, metav1.UpdateOptions{})
		return err
	}
	if err := applyDefaultResourceQuota(ctx, cset, project); err != nil {
		return err
	}
	if err := applyDefaultLimitRange(ctx, cset, project); err != nil {
		return err
	}
	return nil
}

// ProjectAccessMember represents a single subject→role binding for a project.
type ProjectAccessMember struct {
	Subject string
	Role    string
}

// EnsureProjectAccess creates or updates Roles and RoleBindings for the given members
// in the project namespace. Best-effort: if kubeconfig cannot be parsed, this is a no-op.
func EnsureProjectAccess(ctx context.Context, kubeconfig []byte, tenant, project string, members []ProjectAccessMember) error {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		// In tests/dev the kubeconfig may be a stub; skip access management.
		return nil
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	ns := project
	roleClient := cset.RbacV1().Roles(ns)
	rbClient := cset.RbacV1().RoleBindings(ns)

	byRole := map[string][]string{}
	for _, m := range members {
		if m.Subject == "" || m.Role == "" {
			continue
		}
		byRole[m.Role] = append(byRole[m.Role], m.Subject)
	}

	roleTypes := []string{"tenantOwner", "projectDev", "readOnly"}
	for _, role := range roleTypes {
		subjects := byRole[role]
		roleName := projectRoleName(tenant, project, role)

		if len(subjects) == 0 {
			// No members for this role: best-effort delete RoleBinding.
			_ = rbClient.Delete(ctx, roleName, metav1.DeleteOptions{})
			continue
		}

		// Ensure Role exists for this project/role combination.
		if _, err := roleClient.Get(ctx, roleName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			r := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleName,
					Namespace: ns,
				},
				Rules: roleRulesFor(role),
			}
			if _, err := roleClient.Create(ctx, r, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
		} else if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		// Build RoleBinding subjects for this role.
		subjs := make([]rbacv1.Subject, 0, len(subjects))
		for _, s := range subjects {
			subjs = append(subjs, rbacv1.Subject{
				Kind:     "User",
				APIGroup: "rbac.authorization.k8s.io",
				Name:     s,
			})
		}

		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: ns,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     roleName,
			},
			Subjects: subjs,
		}
		if _, err := rbClient.Get(ctx, roleName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			if _, err := rbClient.Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
		} else if err == nil {
			if _, err := rbClient.Update(ctx, rb, metav1.UpdateOptions{}); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func projectRoleName(tenant, project, role string) string {
	// Build a base name and then sanitize it to a valid RFC1123 subdomain:
	// lower-case alphanumeric, '-', '.', max 63 chars, starting/ending with alphanumeric.
	base := fmt.Sprintf("kubenova-%s-%s-%s", tenant, project, role)
	base = strings.ToLower(base)
	var b strings.Builder
	for _, ch := range base {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('-')
		}
	}
	name := b.String()
	if name == "" {
		name = "kubenova"
	}
	// Ensure first and last characters are alphanumeric.
	isAlnum := func(c byte) bool {
		return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
	}
	if !isAlnum(name[0]) {
		name = "k" + name
	}
	if !isAlnum(name[len(name)-1]) {
		name = name[:len(name)-1] + "0"
	}
	if len(name) > 63 {
		name = name[:63]
		if !isAlnum(name[len(name)-1]) {
			name = name[:len(name)-1] + "0"
		}
	}
	return name
}

func roleRulesFor(role string) []rbacv1.PolicyRule {
	verbsOwner := []string{"get", "list", "watch", "create", "update", "patch", "delete"}
	verbsDev := []string{"get", "list", "watch", "create", "update", "patch"}
	verbsRead := []string{"get", "list", "watch"}

	var verbs []string
	switch role {
	case "tenantOwner":
		verbs = verbsOwner
	case "projectDev":
		verbs = verbsDev
	case "readOnly":
		verbs = verbsRead
	default:
		verbs = verbsRead
	}

	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "services", "configmaps", "secrets", "persistentvolumeclaims"},
			Verbs:     verbs,
		},
		{
			APIGroups: []string{"apps", "batch"},
			Resources: []string{"deployments", "statefulsets", "jobs", "cronjobs"},
			Verbs:     verbs,
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"networkpolicies"},
			Verbs:     verbs,
		},
	}
}

func applyDefaultResourceQuota(ctx context.Context, cset kubernetes.Interface, namespace string) error {
	raw := strings.TrimSpace(os.Getenv("DEFAULT_NS_RESOURCEQUOTA"))
	if raw == "" {
		return nil
	}
	var hard map[string]string
	if err := json.Unmarshal([]byte(raw), &hard); err != nil {
		return err
	}
	if len(hard) == 0 {
		return nil
	}
	rqClient := cset.CoreV1().ResourceQuotas(namespace)
	name := "kubenova-default-quota"
	rq, err := rqClient.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		rq = &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
	} else if err != nil {
		return err
	}
	if rq.Spec.Hard == nil {
		rq.Spec.Hard = corev1.ResourceList{}
	}
	for k, v := range hard {
		qty, perr := resource.ParseQuantity(v)
		if perr != nil {
			continue
		}
		rq.Spec.Hard[corev1.ResourceName(k)] = qty
	}
	if rq.CreationTimestamp.IsZero() {
		_, err = rqClient.Create(ctx, rq, metav1.CreateOptions{})
		return err
	}
	_, err = rqClient.Update(ctx, rq, metav1.UpdateOptions{})
	return err
}

func applyDefaultLimitRange(ctx context.Context, cset kubernetes.Interface, namespace string) error {
	raw := strings.TrimSpace(os.Getenv("DEFAULT_PROJECT_QUOTA"))
	if raw == "" {
		return nil
	}
	var max map[string]string
	if err := json.Unmarshal([]byte(raw), &max); err != nil {
		return err
	}
	if len(max) == 0 {
		return nil
	}
	lrClient := cset.CoreV1().LimitRanges(namespace)
	name := "kubenova-default-limits"
	lr, err := lrClient.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		lr = &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
	} else if err != nil {
		return err
	}
	item := corev1.LimitRangeItem{
		Type: corev1.LimitTypeContainer,
		Max:  corev1.ResourceList{},
	}
	for k, v := range max {
		qty, perr := resource.ParseQuantity(v)
		if perr != nil {
			continue
		}
		item.Max[corev1.ResourceName(k)] = qty
	}
	lr.Spec.Limits = []corev1.LimitRangeItem{item}
	if lr.CreationTimestamp.IsZero() {
		_, err = lrClient.Create(ctx, lr, metav1.CreateOptions{})
		return err
	}
	_, err = lrClient.Update(ctx, lr, metav1.UpdateOptions{})
	return err
}
