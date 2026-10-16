{{/*
ACM condition, true unless the cluster is a hosted control plane guest. HyperShift owns a guest's
ClusterVersion and the lifecycle chart upgrades it from the hub, so every converter is inert there.
*/}}
{{- define "upgrade.standalone" -}}
ne (dig "status" "controlPlaneTopology" "" (lookup "config.openshift.io/v1" "Infrastructure" "" "cluster")) "External"
{{- end -}}
