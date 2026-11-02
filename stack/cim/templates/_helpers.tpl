{{- /*
The imageDigestMirrors of every ImageDigestMirrorSet in the disconnected element's
config.clusterResources, as JSON. The hub's values layers carry that config, so cim reads it rather
than restating the site's mirrors.
*/ -}}
{{- define "cim.mirrors" -}}
{{- $mirrors := list }}
{{- range dig "disconnected" "config" "clusterResources" list .Values.stack }}
{{- if eq .kind "ImageDigestMirrorSet" }}{{ $mirrors = concat $mirrors .spec.imageDigestMirrors }}{{ end }}
{{- end }}
{{- toJson $mirrors }}
{{- end }}
