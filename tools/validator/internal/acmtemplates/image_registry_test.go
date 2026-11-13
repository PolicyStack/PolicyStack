package acmtemplates

import (
	"path/filepath"
	"testing"
)

// The OBC outputs and CAs the s3 storage type reads on the managed cluster.
const (
	registryStorage = `apiVersion: v1
kind: ConfigMap
metadata: {name: image-registry-storage, namespace: openshift-image-registry}
data: {BUCKET_NAME: bkt, BUCKET_HOST: s3.openshift-storage.svc, BUCKET_PORT: "443"}`
	registryCredentials = `apiVersion: v1
kind: Secret
metadata: {name: image-registry-storage, namespace: openshift-image-registry}
data: {AWS_ACCESS_KEY_ID: QUs=, AWS_SECRET_ACCESS_KEY: U0s=}`
	serviceCA = `apiVersion: v1
kind: ConfigMap
metadata: {name: openshift-service-ca.crt, namespace: openshift-image-registry}
data: {service-ca.crt: "SVC\n"}`
	ingressCA = `apiVersion: v1
kind: ConfigMap
metadata: {name: default-ingress-cert, namespace: openshift-config-managed}
data: {ca-bundle.crt: "ING\n"}`
	noobaaRoute = `apiVersion: route.openshift.io/v1
kind: Route
metadata: {name: s3, namespace: openshift-storage}
spec: {host: s3-openshift-storage.apps.example}`
)

// TestImageRegistryS3: the registry targets the NooBaa route when it exists, else the bucket's
// service endpoint, and trusts the service CA plus the ingress CA when one is published.
func TestImageRegistryS3(t *testing.T) {
	dir := filepath.Join(repoRoot, "stack/image-registry")
	for _, tt := range []struct {
		name           string
		live           []string
		endpoint, cert string
	}{
		{"route", []string{registryStorage, registryCredentials, serviceCA, ingressCA, noobaaRoute},
			"https://s3-openshift-storage.apps.example", "SVC\nING\n"},
		{"no route or ingress CA", []string{registryStorage, registryCredentials, serviceCA},
			"https://s3.openshift-storage.svc:443", "SVC\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := find(t, resolveTemplate(t, dir, "image-registry-acm-dc1", allOn, "config-registry", tt.live...), "Config", "cluster")
			if got := field(config, "spec", "storage", "s3", "regionEndpoint"); got != tt.endpoint {
				t.Errorf("regionEndpoint %q, want %q", got, tt.endpoint)
			}
			if got := field(config, "spec", "storage", "s3", "bucket"); got != "bkt" {
				t.Errorf("bucket %q", got)
			}
			objs := resolveTemplate(t, dir, "image-registry-acm-dc1", allOn, "s3-credentials", tt.live...)
			secret := find(t, objs, "Secret", "image-registry-private-configuration-user")
			if a, s := field(secret, "data", "REGISTRY_STORAGE_S3_ACCESSKEY"), field(secret, "data", "REGISTRY_STORAGE_S3_SECRETKEY"); a != "QUs=" || s != "U0s=" {
				t.Errorf("credentials %q %q", a, s)
			}
			if got := field(find(t, objs, "ConfigMap", "image-registry-s3-bundle"), "data", "ca-bundle.crt"); got != tt.cert {
				t.Errorf("ca-bundle.crt %q, want %q", got, tt.cert)
			}
		})
	}
}
