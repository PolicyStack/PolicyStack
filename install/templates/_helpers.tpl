{{/*
The policy-library majors this release of the install chart supports. Both ApplicationSets inject
the list into every Application as policystack.libraryMajors, and policy-library fails the render
of a revision whose library major is not in it (policy-library.render). A library major adds its
number here and keeps the previous one, so clusters can move their pins one at a time.
*/}}
{{- define "policystack.libraryMajors" -}}
[4]
{{- end -}}

{{/*
The pointer: ApplicationSet policystack-install, which reads fleet/hubs/<hubName>.yaml and generates
the Application policystack that syncs install/ at the file's installRevision, the default branch
when unset. The root renders it from install/ itself, so it is self-managed: policystack syncs the
ApplicationSet that generates it. A child hub's pointer is enforced by its parent's Policy
policystack-hubs (hubs.yaml), which passes parent so the child doesn't render its own.
Deleting the ApplicationSet or the Application leaves the install in place: the Application has no
resources-finalizer, and every install object carries Prune=false,Delete=false.
Params: root (.), hubName, parent.
*/}}
{{- define "policystack.pointer" -}}
{{- $v := .root.Values -}}
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: policystack-install
  namespace: openshift-gitops
  labels:
    velero.io/exclude-from-backup: "true"
  annotations:
    argocd.argoproj.io/sync-options: Prune=false,Delete=false
spec:
  syncPolicy:
    preserveResourcesOnDeletion: true
  goTemplate: true
  goTemplateOptions: ["missingkey=error"]
  generators:
    - git:
        repoURL: {{ $v.gitRepo | quote }}
        revision: HEAD
        files:
          - path: 'fleet/hubs/{{ .hubName }}.yaml'
  template:
    metadata:
      name: policystack
      labels:
        velero.io/exclude-from-backup: "true"
    spec:
      project: default
      source:
        repoURL: {{ $v.gitRepo | quote }}
        targetRevision: '{{ "{{" }} dig "installRevision" "HEAD" . {{ "}}" }}'
        path: install
        helm:
          releaseName: policystack
          valuesObject:
            gitRepo: {{ $v.gitRepo | quote }}
            hubName: '{{ .hubName }}'
            {{- with $v.repoSecret }}
            repoSecret: {{ . | quote }}
            {{- end }}
            {{- with .parent }}
            parent: {{ . | quote }}
            {{- end }}
      destination:
        server: https://kubernetes.default.svc
        namespace: openshift-gitops
      syncPolicy:
        automated:
          prune: false
          selfHeal: true
        retry:
          limit: -1
          backoff:
            duration: 30s
            factor: 2
            maxDuration: 10m
{{- end -}}
