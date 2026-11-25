package acmtemplates

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

var acmDir = filepath.Join(repoRoot, "stack/acm")

const obsNS = "open-cluster-management-observability"

// acmRelease carries a cluster suffix so observability's element: odf dependency resolves.
const acmRelease = "acm-local-cluster"

// obsSecret is a live Secret in the observability namespace with data base64-encoded.
func obsSecret(name string, data map[string]string) string {
	enc := map[string]string{}
	for k, v := range data {
		enc[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	out, _ := json.Marshal(map[string]any{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]string{"name": name, "namespace": obsNS},
		"data":     enc,
	})
	return string(out)
}

const pullSecret = `apiVersion: v1
kind: Secret
metadata: {name: pull-secret, namespace: openshift-config}
data: {.dockerconfigjson: e30=}`

// observability resolves observability-mco for the s3 config (a YAML flow map, empty for NooBaa)
// and returns the parsed thanos.yaml config and the MultiClusterObservability metricObjectStorage.
func observability(t *testing.T, s3 string, live ...string) (thanos, storage map[string]any) {
	t.Helper()
	files := valuesFile(t, "stack: {acm: {config: {observability: {s3: "+s3+"}}}}\n")
	raw := renderRaw(t, acmDir, acmRelease, files)["observability-mco"]
	objs := resolve(t, raw, append(live, pullSecret)...)
	var doc struct {
		Config map[string]any `json:"config"`
	}
	if err := yaml.Unmarshal([]byte(field(find(t, objs, "Secret", "thanos-object-storage"), "stringData", "thanos.yaml")), &doc); err != nil {
		t.Fatal(err)
	}
	mos, _ := nested(find(t, objs, "MultiClusterObservability", "observability"), "spec", "storageConfig", "metricObjectStorage").(map[string]any)
	return doc.Config, mos
}

// TestObservabilityNooBaa: without s3 the Thanos config is read from the ObjectBucketClaim's
// ConfigMap and Secret and trusts the service CA, as before s3 existed.
func TestObservabilityNooBaa(t *testing.T) {
	cm := `apiVersion: v1
kind: ConfigMap
metadata: {name: acm-observability, namespace: ` + obsNS + `}
data: {BUCKET_NAME: acm-observability-1a2b, BUCKET_HOST: s3.openshift-storage.svc, BUCKET_PORT: "443"}`
	sec := obsSecret("acm-observability", map[string]string{"AWS_ACCESS_KEY_ID": "id", "AWS_SECRET_ACCESS_KEY": "key"})
	thanos, mos := observability(t, "{}", cm, sec)
	wantYAML(t, thanos, `
bucket: acm-observability-1a2b
endpoint: s3.openshift-storage.svc:443
insecure: false
access_key: id
secret_key: key
http_config: {tls_config: {ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt}}`)
	wantYAML(t, mos, `{name: thanos-object-storage, key: thanos.yaml}`)
}

// TestObservabilityS3: the Thanos config comes from s3, credentials of any content survive, and
// caSecret is both mounted and named in ca_file.
func TestObservabilityS3(t *testing.T) {
	for _, tt := range []struct {
		name, s3, id, key, thanos, storage string
	}{
		{"minimal", `{bucket: metrics, endpoint: "s3.us-east-1.amazonaws.com", credentialsSecret: creds}`, "AKIA", "plain",
			`{bucket: metrics, endpoint: s3.us-east-1.amazonaws.com, insecure: false, access_key: AKIA, secret_key: plain}`,
			`{name: thanos-object-storage, key: thanos.yaml}`},
		{"region and plain HTTP", `{bucket: metrics, endpoint: "minio:9000", region: eu-west-1, insecure: true, credentialsSecret: creds}`, "id", "key",
			`{bucket: metrics, endpoint: "minio:9000", region: eu-west-1, insecure: true, access_key: id, secret_key: key}`,
			`{name: thanos-object-storage, key: thanos.yaml}`},
		{"private CA", `{bucket: metrics, endpoint: "minio.example.com:9000", credentialsSecret: creds, caSecret: s3-ca}`, "id", "key",
			`{bucket: metrics, endpoint: "minio.example.com:9000", insecure: false, access_key: id, secret_key: key,
			  http_config: {tls_config: {ca_file: /etc/s3/certs/ca.crt}}}`,
			`{name: thanos-object-storage, key: thanos.yaml, tlsSecretName: s3-ca, tlsSecretMountPath: /etc/s3/certs}`},
		{"YAML-significant credentials", `{bucket: metrics, endpoint: "minio:9000", credentialsSecret: creds}`, "*anchor", `it's a: b #c "q" \x`,
			`{bucket: metrics, endpoint: "minio:9000", insecure: false, access_key: "*anchor", secret_key: "it's a: b #c \"q\" \\x"}`,
			`{name: thanos-object-storage, key: thanos.yaml}`},
		{"caSecret without bucket", `{caSecret: s3-ca}`, "", "", "", `{name: thanos-object-storage, key: thanos.yaml}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.thanos == "" {
				_, mos := observability(t, tt.s3, `{apiVersion: v1, kind: ConfigMap, metadata: {name: acm-observability, namespace: `+obsNS+`}, data: {BUCKET_NAME: b, BUCKET_HOST: h, BUCKET_PORT: "1"}}`,
					obsSecret("acm-observability", map[string]string{"AWS_ACCESS_KEY_ID": "i", "AWS_SECRET_ACCESS_KEY": "k"}))
				wantYAML(t, mos, tt.storage)
				return
			}
			thanos, mos := observability(t, tt.s3, obsSecret("creds", map[string]string{"AWS_ACCESS_KEY_ID": tt.id, "AWS_SECRET_ACCESS_KEY": tt.key}))
			wantYAML(t, thanos, tt.thanos)
			wantYAML(t, mos, tt.storage)
		})
	}
}

// TestObservabilityS3Bucket: with s3 the bucket template creates only the Namespace.
func TestObservabilityS3Bucket(t *testing.T) {
	for s3, want := range map[string][]string{
		"{}": {"Namespace", "ObjectBucketClaim"},
		`{bucket: metrics, endpoint: "minio:9000", credentialsSecret: creds}`: {"Namespace"},
	} {
		files := valuesFile(t, "stack: {acm: {config: {observability: {s3: "+s3+"}}}}\n")
		var got []string
		for _, o := range resolveTemplate(t, acmDir, acmRelease, files, "observability-bucket") {
			got = append(got, o.ObjectDefinition.GetKind())
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("s3 %s: bucket renders %v, want %v", s3, got, want)
		}
	}
}

// TestObservabilityS3Rejected: s3 without its endpoint or credentials fails the render rather than
// a MultiClusterObservability the webhook rejects.
func TestObservabilityS3Rejected(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(context.Background(), acmDir); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, s3, want string }{
		{"no endpoint", `{bucket: metrics, credentialsSecret: creds}`, "s3.endpoint is required"},
		{"no credentials", `{bucket: metrics, endpoint: "minio:9000"}`, "s3.credentialsSecret is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := valuesFile(t, "stack: {acm: {config: {observability: {s3: "+tt.s3+"}}}}\n")
			tr := runner.Template(context.Background(), acmDir, acmRelease, files, nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), tt.want) {
				t.Fatalf("want %q, got err %v: %s", tt.want, tr.Err, tr.Stderr)
			}
		})
	}
}

// TestObservabilityCustomMetrics: metrics_list.yaml holds the names, and each recording rule comes
// back as written after the collector's JSON round trip (metrics_collector.go formats
// {"name":"%s","query":"%s"} and unmarshals it).
func TestObservabilityCustomMetrics(t *testing.T) {
	const expr = `histogram_quantile(0.90, sum(rate(apiserver_request_duration_seconds_bucket{job="apiserver", verb!~"WATCH|a\\.b"}[5m])) by (verb, le)) < 5 & 1`
	for _, tt := range []struct {
		name, metrics string
		names         []string
		rules         map[string]string
	}{
		{"empty", "{}", []string{}, map[string]string{}},
		{"names and rules", `{names: [node_memory_MemTotal_bytes], recordingRules: [{record: "apiserver:p90", expr: '` + expr + `'}, {record: up:sum, expr: sum(up)}]}`,
			[]string{"node_memory_MemTotal_bytes"}, map[string]string{"apiserver:p90": expr, "up:sum": "sum(up)"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := valuesFile(t, "stack: {acm: {config: {observability: {customMetrics: "+tt.metrics+"}}}}\n")
			objs := resolveTemplate(t, acmDir, acmRelease, files, "observability-metrics")
			cm := find(t, objs, "ConfigMap", "observability-metrics-custom-allowlist")
			if cm.ComplianceType != "mustonlyhave" || field(cm, "metadata", "namespace") != obsNS {
				t.Errorf("complianceType %q namespace %q", cm.ComplianceType, field(cm, "metadata", "namespace"))
			}
			var list struct {
				Names          []string `json:"names"`
				RecordingRules []struct {
					Record string `json:"record"`
					Expr   string `json:"expr"`
				} `json:"recording_rules"`
			}
			if err := yaml.Unmarshal([]byte(field(cm, "data", "metrics_list.yaml")), &list); err != nil {
				t.Fatal(err)
			}
			if list.Names == nil || list.RecordingRules == nil {
				t.Errorf("names and recording_rules must both be present: %+v", list)
			}
			wantYAML(t, list.Names, asYAML(t, tt.names))
			got := map[string]string{}
			for _, r := range list.RecordingRules {
				var rule struct{ Name, Query string }
				if err := json.Unmarshal(fmt.Appendf(nil, `{"name":"%s","query":"%s"}`, r.Record, r.Expr), &rule); err != nil {
					t.Fatalf("collector cannot parse rule %s: %v", r.Record, err)
				}
				got[rule.Name] = rule.Query
			}
			wantYAML(t, got, asYAML(t, tt.rules))
		})
	}
}
