{{/*
The cluster this release builds: the Application is lifecycle-<cluster>, and Argo CD uses that as
the Helm release name.
*/}}
{{- define "lifecycle.cluster" -}}
{{- include "policy-library.clusterSuffix" . | required (printf "lifecycle: release %q must be named lifecycle-<cluster>" .Release.Name) -}}
{{- end -}}

{{/*
Annotations hypershift-addon puts on the ManagedCluster it auto-imports for a HostedCluster hosted
on this hub. A ManagedCluster without them was not built here, so lifecycle never builds over it or
detaches it. The hub template resolves to the hub's own ManagedCluster name.
*/}}
{{- define "lifecycle.ownerAnnotations" -}}
open-cluster-management/created-via: hypershift
import.open-cluster-management.io/hosting-cluster-name: '{{ "{{hub" }} .ManagedClusterName {{ "hub}}" }}'
{{- end -}}
