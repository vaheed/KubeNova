package cluster

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	appClusterRoleName              = "kubenova-app-reader"
	sandboxClusterRoleName          = "kubenova-sandbox-editor"
	appNamespaceRoleBindingName     = "kubenova-app-reader-binding"
	sandboxNamespaceRoleBindingName = "kubenova-sandbox-editor-binding"
)

var appClusterRoleRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{""},
		Resources: []string{"pods", "pods/log", "pods/exec", "services", "configmaps", "secrets", "persistentvolumeclaims"},
		Verbs:     []string{"get", "list", "watch"},
	},
	{
		APIGroups: []string{"apps", "batch"},
		Resources: []string{"deployments", "statefulsets", "jobs", "cronjobs"},
		Verbs:     []string{"get", "list", "watch"},
	},
	{
		APIGroups: []string{"networking.k8s.io"},
		Resources: []string{"networkpolicies"},
		Verbs:     []string{"get", "list", "watch"},
	},
}

var sandboxClusterRoleRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{""},
		Resources: []string{"pods", "pods/log", "pods/exec", "services", "configmaps", "secrets", "persistentvolumeclaims"},
		Verbs:     []string{"*"},
	},
	{
		APIGroups: []string{"apps", "batch"},
		Resources: []string{"deployments", "statefulsets", "jobs", "cronjobs"},
		Verbs:     []string{"*"},
	},
	{
		APIGroups: []string{"networking.k8s.io"},
		Resources: []string{"networkpolicies"},
		Verbs:     []string{"*"},
	},
}

func ensureAppClusterRole(ctx context.Context, cset kubernetes.Interface) error {
	return ensureClusterRole(ctx, cset, appClusterRoleName, appClusterRoleRules)
}

func ensureSandboxClusterRole(ctx context.Context, cset kubernetes.Interface) error {
	return ensureClusterRole(ctx, cset, sandboxClusterRoleName, sandboxClusterRoleRules)
}

func ensureClusterRole(ctx context.Context, cset kubernetes.Interface, name string, rules []rbacv1.PolicyRule) error {
	crClient := cset.RbacV1().ClusterRoles()
	if cr, err := crClient.Get(ctx, name, metav1.GetOptions{}); err == nil {
		cr.Rules = rules
		_, err = crClient.Update(ctx, cr, metav1.UpdateOptions{})
		if err != nil && !apierrors.IsConflict(err) {
			return err
		}
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	cr := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: rules}
	_, err := crClient.Create(ctx, cr, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func ensureNamespaceGroupBinding(ctx context.Context, cset kubernetes.Interface, namespace, bindingName, clusterRole, group string) error {
	if namespace == "" || group == "" {
		return nil
	}
	rbClient := cset.RbacV1().RoleBindings(namespace)
	subjects := []rbacv1.Subject{{
		Kind:     "Group",
		APIGroup: "rbac.authorization.k8s.io",
		Name:     group,
	}}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName, Namespace: namespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
		Subjects: subjects,
	}
	if existing, err := rbClient.Get(ctx, bindingName, metav1.GetOptions{}); err == nil {
		existing.Subjects = subjects
		existing.RoleRef = rb.RoleRef
		if _, err := rbClient.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return err
		}
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	if _, err := rbClient.Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
