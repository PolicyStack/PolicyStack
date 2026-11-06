{{/*
The policy-library majors this release of the install chart supports. Both ApplicationSets inject
the list into every Application as policystack.libraryMajors, and policy-library fails the render
of a revision whose library major is not in it (policy-library.render). A library major adds its
number here and keeps the previous one, so clusters can move their pins one at a time.
*/}}
{{- define "policystack.libraryMajors" -}}
[4]
{{- end -}}
