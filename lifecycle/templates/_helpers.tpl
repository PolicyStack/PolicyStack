{{/*
The cluster this release builds: the Application is lifecycle-<cluster>, and Argo CD uses that as
the Helm release name.
*/}}
{{- define "lifecycle.cluster" -}}
{{- include "policy-library.clusterSuffix" . | required (printf "lifecycle: release %q must be named lifecycle-<cluster>" .Release.Name) -}}
{{- end -}}

{{/*
Annotations that mark a ManagedCluster as built by this hub. A ManagedCluster without them was not
built here, so lifecycle never builds over it or detaches it.
- hypershift: set by the hypershift-addon when it auto-imports a HostedCluster hosted on this hub.
  The hub template resolves to the hub's own ManagedCluster name.
- hive: set by the hive engine on the ManagedCluster it creates.
- siteconfig: set by SiteConfig on the ManagedCluster it renders, from the ClusterInstance's
  extraAnnotations.
*/}}
{{- define "lifecycle.ownerAnnotations" -}}
{{- if eq .Values.install.engine "hypershift" -}}
open-cluster-management/created-via: hypershift
import.open-cluster-management.io/hosting-cluster-name: '{{ "{{hub" }} .ManagedClusterName {{ "hub}}" }}'
{{- else -}}
lifecycle.policystk.io/hub: {{ .Values.hub | quote }}
{{- end -}}
{{- end -}}

{{/*
<releaseRepository>:<version>-multi, the release every engine installs.
*/}}
{{- define "lifecycle.releaseImage" -}}
{{- printf "%s:%s-multi" .Values.install.releaseRepository (required "version is required" .Values.version) -}}
{{- end -}}

{{/*
The console credential's namespace and name, quoted, as fromSecret takes them.
*/}}
{{- define "lifecycle.credential" -}}
{{- $cred := .Values.install.credential -}}
{{- printf "%q %q" (required "install.credential.namespace is required" $cred.namespace) (required "install.credential.name is required" $cred.name) -}}
{{- end -}}

{{/*
<cluster>-pull-secret in namespace, copied from the credential's pullSecret key.
Args: dict "root" <root context> "namespace" <namespace>
*/}}
{{- define "lifecycle.pullSecret" -}}
- complianceType: musthave
  objectDefinition:
    apiVersion: v1
    kind: Secret
    type: kubernetes.io/dockerconfigjson
    metadata:
      name: {{ include "lifecycle.cluster" .root }}-pull-secret
      namespace: {{ .namespace }}
    data:
      .dockerconfigjson: '{{ "{{" }} fromSecret {{ include "lifecycle.credential" .root }} "pullSecret" {{ "}}" }}'
{{- end -}}

{{/*
Deletes install.manifests. Each engine's cleanup converter ends with it, once the cluster is gone.
*/}}
{{- define "lifecycle.cleanupManifests" -}}
{{- range .Values.install.manifests }}
- complianceType: mustnothave
  objectDefinition:
    apiVersion: {{ .apiVersion }}
    kind: {{ .kind }}
    metadata: {{- pick .metadata "name" "namespace" | toYaml | nindent 6 }}
{{- end }}
{{- end -}}

{{/*
The single key of install.hive.platform: aws, gcp or azure.
*/}}
{{- define "lifecycle.hivePlatform" -}}
{{- $keys := keys (.Values.install.hive.platform | default dict) -}}
{{- if ne (len $keys) 1 }}{{ fail "install.hive.platform must set exactly one of aws, gcp or azure" }}{{ end -}}
{{- first $keys -}}
{{- end -}}
