package metrics_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Scenario "Chart": the collector scrapes gateway, core and worker every 2 s and the others every
// 15 s, keeps 6 hours, and may only read the pods of its own namespace.
func TestPrometheusInTheChart(t *testing.T) {
	tpl := read(t, "deploy/helm/eps/templates/prometheus.yaml")
	values := read(t, "deploy/helm/eps/values.yaml")
	for _, want := range []string{`(dict "name" "onsale" "interval" "2s")`, `(dict "name" "others" "interval" "15s")`, "kind: Role\n",
		`resources: ["pods"]`, `verbs: ["get", "list", "watch"]`, "names: [{{ $.Release.Namespace | quote }}]", "--storage.tsdb.retention.time={{ .Values.prometheus.retention }}",
		"readOnlyRootFilesystem: true", "runAsNonRoot: true"} {
		if !strings.Contains(tpl, want) {
			t.Errorf("prometheus.yaml lacks %q", want)
		}
	}
	if strings.Contains(tpl, "kind: ClusterRole") || strings.Contains(tpl, "kind: Ingress") {
		t.Error("the collector must have a namespace role and no ingress")
	}
	if !regexp.MustCompile(`(?m)^prometheus:\n(?:  .*\n)*  retention: 6h`).MatchString(values) {
		t.Error("retention is not 6h")
	}
	for _, svc := range []string{"core", "gateway", "worker"} {
		block := regexp.MustCompile(`(?ms)^` + svc + `:\n(.*?)^\S`).FindStringSubmatch(values)
		if block == nil || !strings.Contains(block[1], "scrapeInterval: 2s") {
			t.Errorf("%s is not scraped every 2 s", svc)
		}
	}
	if _, err := exec.LookPath("helm"); err != nil {
		t.Log("helm not installed: the rendered chart is checked by CI's helm job")
		return
	}
	out, err := exec.Command("helm", "template", "eps-ci", filepath.Join("..", "..", "..", "deploy", "helm", "eps"), "--namespace", "eps-ci").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	for _, want := range []string{"scrape_interval: 2s", "scrape_interval: 15s", "--storage.tsdb.retention.time=6h", `names: ["eps-ci"]`, `eps.dev/scrape-interval: "2s"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("rendered chart lacks %q", want)
		}
	}
}
