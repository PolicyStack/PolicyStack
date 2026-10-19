package checks

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy022_ConfigReads(t *testing.T) {
	const values = `
stack:
  myElement:
    config:
      fstype: xfs
      data: {}
      observability: {}
`
	tests := []struct {
		name      string
		values    string // default values
		converter string
		want      []string // "<line>:<col> <key>"
	}{
		{
			name: "defined keys are silent",
			converter: `{{- $cfg := .Values.stack.myElement.config }}
fstype: {{ $cfg.fstype }}
{{- with .Values.stack.myElement.config.data }}
data: {{ toYaml . }}
{{- end }}
other: {{ $.Values.stack.myElement.config.fstype | quote }}
`,
		},
		{
			name: "bound variable",
			converter: `{{- $cfg := .Values.stack.myElement.config }}
fstype: {{ $cfg.fsytpe }}
`,
			want: []string{"2:17 fsytpe"},
		},
		{
			name:      "any variable name, any pipeline position",
			converter: "{{- $spec := .Values.stack.myElement.config -}}\nx: {{ default \"a\" $spec.mode | quote }}\n",
			want:      []string{"2:25 mode"},
		},
		{
			name:      "direct read",
			converter: "a: b\nx: '{{ .Values.stack.myElement.config.uiPlugn }}'\n",
			want:      []string{"2:39 uiPlugn"},
		},
		{
			name:      "direct read from $",
			converter: "x: {{ toYaml $.Values.stack.myElement.config.dat }}\n",
			want:      []string{"1:46 dat"},
		},
		{
			name:      "parenthesised read",
			converter: "x: {{ (index .Values.stack.myElement.config.typo 0) }}\n",
			want:      []string{"1:45 typo"},
		},
		{
			name: "nested keys are out of scope",
			converter: `{{- $cfg := .Values.stack.myElement.config }}
{{- $o := .Values.stack.myElement.config.observability }}
a: {{ $cfg.data.anything }}
b: {{ .Values.stack.myElement.config.data.anything }}
c: {{ $o.anything }}
`,
		},
		{
			name:      "another element's config is out of scope",
			converter: "x: {{ .Values.stack.other.config.anything }}\n",
		},
		{
			name: "escaped ACM template and comments are not Helm reads",
			converter: `{{- /* read .Values.stack.myElement.config.typo1 */ -}}
{{- $cfg := .Values.stack.myElement.config }}
a: '{{ "{{" }} $cfg.typo2 {{ "}}" }}'
b: '{{ "{{" }} printf "%s" {{ $cfg.typo3 | quote }} {{ "}}" }}'
`,
			want: []string{"4:36 typo3"},
		},
		{
			name: "if and else branches",
			converter: `{{- $cfg := .Values.stack.myElement.config }}
{{- if $cfg.ifTypo }}
a: {{ $cfg.thenTypo }}
{{- else if $cfg.fstype }}
b: {{ $cfg.elseTypo }}
{{- end }}
`,
			want: []string{"2:13 ifTypo", "3:12 thenTypo", "5:12 elseTypo"},
		},
		{
			name: "with binds in its block only",
			converter: `{{- with $c := .Values.stack.myElement.config }}
a: {{ $c.inner }}
{{- end }}
{{- $c := dict }}
b: {{ $c.outer }}
`,
			want: []string{"2:10 inner"},
		},
		{
			name: "range declarations bind elements",
			converter: `{{- $cfg := .Values.stack.myElement.config }}
{{- range $k, $cfg := .Values.stack.myElement.config }}
a: {{ $cfg.elem }}
{{- end }}
b: {{ $cfg.after }}
`,
			want: []string{"5:12 after"},
		},
		{
			name: "range over the config reads no key",
			converter: `{{- range $k, $v := .Values.stack.myElement.config }}
{{ $k }}: {{ $v.anything }}
{{- end }}
`,
		},
		{
			name: "reassignment unbinds",
			converter: `{{- $cfg := .Values.stack.myElement.config }}
a: {{ $cfg.before }}
{{- $cfg = dict "x" 1 }}
b: {{ $cfg.x }}
`,
			want: []string{"2:12 before"},
		},
		{
			name: "piped binding is not the config",
			converter: `{{- $cfg := .Values.stack.myElement.config | toYaml }}
a: {{ $cfg.x }}
`,
		},
		{
			name:      "define blocks",
			converter: "{{- define \"x\" }}{{ $c := .Values.stack.myElement.config }}{{ $c.inDefine }}{{ end }}\n",
			want:      []string{"1:66 inDefine"},
		},
		{
			name:      "element without config",
			values:    "stack:\n  myElement:\n    enabled: true\n",
			converter: "x: {{ .Values.stack.myElement.config.fstype }}\n",
			want:      []string{"1:38 fstype"},
		},
		{
			name:      "unparsable converter is left to RENDER000",
			converter: "x: {{ .Values.stack.myElement.config.typo }\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			el := loadElement(t, cmp.Or(tt.values, values), "", nil)
			writeConverters(t, el, map[string]string{"c.yaml": tt.converter})
			var got []string
			for _, f := range (&ConfigReadCheck{}).Run(Context{Element: el}) {
				if f.RuleID != "POLICY022" || f.Severity != SevError || f.File != filepath.Join(el.ConvertersDir, "c.yaml") {
					t.Errorf("unexpected finding %+v", f)
				}
				got = append(got, fmt.Sprintf("%d:%d %s", f.Line, f.Col, f.Message))
			}
			var want []string
			for _, w := range tt.want {
				var pos, key string
				fmt.Sscan(w, &pos, &key)
				want = append(want, fmt.Sprintf("%s reads stack.myElement.config.%s, which is not in the element's values.yaml", pos, key))
			}
			if !slices.Equal(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestPolicy022_Components checks that each lifecycle component reports only reads of its own
// config from the converters they share.
func TestPolicy022_Components(t *testing.T) {
	const schema = `{"properties":{"install":{"properties":{"engine":{"enum":["a","b"]}}}}}`
	el := loadElement(t, `
stack:
  a:
    config: {x: 1}
  b:
    config: {y: 1}
`, "apiVersion: v2\nname: lifecycle\nversion: 0.1.0\n", nil)
	writeConverters(t, el, map[string]string{
		"a/c.yaml": "{{ .Values.stack.a.config.x }}{{ .Values.stack.a.config.y }}{{ .Values.stack.b.config.x }}\n",
	})
	dir := filepath.Dir(el.ValuesFile)
	if err := os.WriteFile(filepath.Join(dir, "values.schema.json"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	comps, err := chart.LoadComponents(dir, "policy")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, el := range comps {
		for _, f := range (&ConfigReadCheck{}).Run(Context{Element: el}) {
			got = append(got, fmt.Sprintf("%s %d:%d", el.StackKey, f.Line, f.Col))
		}
	}
	if want := []string{"a 1:57", "b 1:87"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// writeConverters writes each converter of el, by path under converters/.
func writeConverters(t *testing.T, el *chart.Element, converters map[string]string) {
	t.Helper()
	for name, body := range converters {
		p := filepath.Join(el.ConvertersDir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
