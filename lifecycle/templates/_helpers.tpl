{{/*
The cluster this release builds: the Application is lifecycle-<cluster>, and Argo CD uses that as
the Helm release name.
*/}}
{{- define "lifecycle.cluster" -}}
{{- include "policy-library.clusterSuffix" . | required (printf "lifecycle: release %q must be named lifecycle-<cluster>" .Release.Name) -}}
{{- end -}}

{{/*
The marker on the engine's root object, written in the musthave that creates it, and on the
ManagedCluster hive and siteconfig create. Ownership is the key's presence; the value records the
hub that last reconciled it, so a restored or migrated object stays owned.
*/}}
{{- define "lifecycle.marker" -}}
lifecycle.policystk.io/hub: {{ .Values.hub | quote }}
{{- end -}}

{{/*
The engine's root object by identity alone, from converters/<engine>/root.yaml: the object
lifecycle creates first and deletes to destroy the cluster.
*/}}
{{- define "lifecycle.root" -}}
{{- tpl (.Files.Get (printf "converters/%s/root.yaml" .Values.install.engine)) . -}}
{{- end -}}

{{/*
ACM template text setting $foreign to the object named <cluster> lifecycle did not build, else to
an empty map: the first that exists of the root object, the ManagedCluster (an imported cluster)
and, for siteconfig, the ClusterDeployment SiteConfig would adopt, unless it carries the marker.
*/}}
{{- define "lifecycle.foreign" -}}
{{- $c := include "lifecycle.cluster" . | quote }}
{{- $r := include "lifecycle.root" . | fromYaml }}
{{ "{{-" }} $foreign := lookup {{ $r.apiVersion | quote }} {{ $r.kind | quote }} {{ $r.metadata.namespace | quote }} {{ $c }} {{ "}}" }}
{{ "{{-" }} if not $foreign {{ "}}" }}{{ "{{-" }} $foreign = lookup "cluster.open-cluster-management.io/v1" "ManagedCluster" "" {{ $c }} {{ "}}" }}{{ "{{-" }} end {{ "}}" }}
{{- if eq .Values.install.engine "siteconfig" }}
{{ "{{-" }} if not $foreign {{ "}}" }}{{ "{{-" }} $foreign = lookup "hive.openshift.io/v1" "ClusterDeployment" {{ $c }} {{ $c }} {{ "}}" }}{{ "{{-" }} end {{ "}}" }}
{{- end }}
{{ "{{-" }} if hasKey (dig "metadata" "annotations" dict $foreign) "lifecycle.policystk.io/hub" {{ "}}" }}{{ "{{-" }} $foreign = dict {{ "}}" }}{{ "{{-" }} end {{ "}}" }}
{{- end -}}

{{/*
Fails the template while lifecycle.foreign finds something, so a step that writes or deletes never
acts on an object it did not build, and the chain stops NonCompliant. Every enforced step but
manifests starts with it: the check runs in the same evaluation as the write.
*/}}
{{- define "lifecycle.refuse" -}}
{{ include "lifecycle.foreign" . }}
{{ "{{-" }} with $foreign {{ "}}" }}{{ "{{" }} fail (printf "%s %s was not built by lifecycle (no lifecycle.policystk.io/hub annotation); refusing" .kind .metadata.name) {{ "}}" }}{{ "{{-" }} end {{ "}}" }}
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
The single key of install.hive.platform: aws, gcp, azure or vsphere.
*/}}
{{- define "lifecycle.hivePlatform" -}}
{{- $keys := keys (.Values.install.hive.platform | default dict) -}}
{{- if ne (len $keys) 1 }}{{ fail "install.hive.platform must set exactly one of aws, gcp, azure or vsphere" }}{{ end -}}
{{- first $keys -}}
{{- end -}}
