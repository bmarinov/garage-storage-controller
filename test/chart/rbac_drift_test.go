//go:build chart

package chart_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

func TestClusterRoleMatchesConfig(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Fatal("helm not found in PATH")
	}

	_, tFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(tFile), "..", "..")

	sourceRules := mustParseClusterRole(t, filepath.Join(repoRoot, "config", "rbac", "role.yaml"))
	chartRules := mustRenderChartClusterRoleRules(t, repoRoot)

	var missing []rbacv1.PolicyRule
	for _, want := range sourceRules {
		if !ruleInSlice(want, chartRules) {
			missing = append(missing, want)
		}
	}

	if len(missing) > 0 {
		var b bytes.Buffer
		for _, r := range missing {
			fmt.Fprintf(&b, "apiGroups=%v resources=%v verbs=%v\n", r.APIGroups, r.Resources, r.Verbs)
		}
		t.Errorf("chart/templates/rbac.yaml ClusterRole is missing %d rule(s) from config/rbac/role.yaml:\n%s"+
			"Update chart/templates/rbac.yaml ClusterRole rules to match config/rbac/role.yaml.",
			len(missing), b.String())
	}
}

func mustParseClusterRole(t *testing.T, path string) []rbacv1.PolicyRule {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var cr rbacv1.ClusterRole
	if err := yaml.Unmarshal(data, &cr); err != nil {
		t.Fatalf("parsing ClusterRole from %s: %v", path, err)
	}
	return cr.Rules
}

func mustRenderChartClusterRoleRules(t *testing.T, repoRoot string) []rbacv1.PolicyRule {
	t.Helper()
	chartDir := filepath.Join(repoRoot, "chart")
	cmd := exec.Command("helm", "template", chartDir, "--values", filepath.Join(chartDir, "values.test.yaml"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helm template: %v", err)
	}
	for doc := range strings.SplitSeq(string(out), "---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var meta struct {
			Kind string `json:"kind"`
		}
		if err := yaml.Unmarshal([]byte(doc), &meta); err != nil || meta.Kind != "ClusterRole" {
			continue
		}
		var cr rbacv1.ClusterRole
		if err := yaml.Unmarshal([]byte(doc), &cr); err != nil {
			t.Fatalf("parsing ClusterRole from helm template output: %v", err)
		}
		return cr.Rules
	}
	t.Fatal("ClusterRole not found in helm template output")
	return nil
}

func ruleInSlice(target rbacv1.PolicyRule, rules []rbacv1.PolicyRule) bool {
	nt := normalizeRule(target)
	for _, r := range rules {
		if rulesEqual(normalizeRule(r), nt) {
			return true
		}
	}
	return false
}

func normalizeRule(r rbacv1.PolicyRule) rbacv1.PolicyRule {
	r.APIGroups = sortedCopy(r.APIGroups)
	r.Resources = sortedCopy(r.Resources)
	r.Verbs = sortedCopy(r.Verbs)
	return r
}

func sortedCopy(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

func rulesEqual(a, b rbacv1.PolicyRule) bool {
	return strSlicesEqual(a.APIGroups, b.APIGroups) &&
		strSlicesEqual(a.Resources, b.Resources) &&
		strSlicesEqual(a.Verbs, b.Verbs)
}

func strSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
