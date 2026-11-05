package checks

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template/parse"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// ConfigReadCheck (POLICY022) reports a converter reading a top-level config key the element's
// values.yaml does not define: Helm renders it as null without complaint. It recognises
// `.Config.<key>`, `.Values.stack.<el>.config.<key>`, both from `$`, and `$v.<key>` where `$v` is
// bound to either in scope. Both read stack.<el>.config; .Config also holds the config of the
// templates rendering the converter, or, in a partial no template renders, of any template. Only
// Helm actions are read, so an escaped ACM template, text to Helm, is skipped. Each lifecycle
// component checks its own config.
type ConfigReadCheck struct{}

func (ConfigReadCheck) ID() string   { return "POLICY022" }
func (ConfigReadCheck) Phase() Phase { return PhaseChart }

func (c *ConfigReadCheck) Run(ctx Context) []Finding {
	el := ctx.Element
	if el == nil || el.StackKey == "" {
		return nil
	}
	elementKeys := configKeys(el)
	var out []Finding
	converters := os.DirFS(el.ConvertersDir)
	_ = fs.WalkDir(converters, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(converters, rel)
		if err != nil {
			return nil
		}
		path := filepath.Join(el.ConvertersDir, rel)
		text := string(data)
		conv := strings.TrimSuffix(filepath.ToSlash(rel), ".yaml")
		own, rendered := templateConfigKeys(el, el.Values.Component, conv)
		// A converter only a sibling lifecycle engine renders reads that engine's .Config.
		if !rendered && slices.ContainsFunc(el.Components, func(nc chart.NamedComponent) bool {
			_, ok := templateConfigKeys(el, nc.Component, conv)
			return ok
		}) {
			return nil
		}
		// A partial pulled in with tpl gets the including template's .Config.
		if !rendered && partial(filepath.ToSlash(rel)) {
			own = allTemplateConfigKeys(el)
		}
		for _, r := range configReadsIn(path, text, el.StackKey) {
			if slices.Contains(elementKeys, r.key) || r.dot && slices.Contains(own, r.key) {
				continue
			}
			before := text[:r.pos]
			out = append(out, Finding{
				RuleID:   c.ID(),
				Severity: SevError,
				Element:  el.ChartName,
				Message:  fmt.Sprintf("reads config key %s, which is not in stack.%s.config of the element's values.yaml or the config of a template rendering it", r.key, el.StackKey),
				File:     path,
				Line:     1 + strings.Count(before, "\n"),
				Col:      r.pos - strings.LastIndex(before, "\n"),
			})
		}
		return nil
	})
	return out
}

// templateConfigKeys returns the top-level keys of templates.<t>.config in values.yaml for each
// template of comp rendering converter, a path under converters/ without .yaml, and whether any does.
func templateConfigKeys(el *chart.Element, comp *chart.Component, converter string) ([]string, bool) {
	var out []string
	rendered := false
	for _, s := range comp.SubPolicies() {
		if slices.Contains(s.Files, converter) {
			rendered = true
			out = append(out, subPolicyConfigKeys(el, s)...)
		}
	}
	return out, rendered
}

// allTemplateConfigKeys returns the top-level keys of every templates.<t>.config of el's component.
func allTemplateConfigKeys(el *chart.Element) []string {
	var out []string
	for _, s := range el.Values.Component.SubPolicies() {
		out = append(out, subPolicyConfigKeys(el, s)...)
	}
	return out
}

func subPolicyConfigKeys(el *chart.Element, s chart.SubPolicy) []string {
	return mapKeys(sourceloc.FindNode(el.ValuesDoc, slices.Concat([]string{"stack", el.StackKey}, s.Path, []string{"config"})...))
}

// configRead is a read of a top-level config key, pos its byte offset in the converter. dot is set
// for a .Config read, which also sees template config.
type configRead struct {
	pos int
	key string
	dot bool
}

// configReadsIn returns the reads of stack.<stackKey>.config keys in a converter. A converter that
// does not parse fails the render, which RENDER000 reports.
func configReadsIn(name, text, stackKey string) []configRead {
	t := parse.New(name)
	t.Mode = parse.SkipFuncCheck
	trees := map[string]*parse.Tree{}
	if _, err := t.Parse(text, "", "", trees); err != nil {
		return nil
	}
	w := configWalker{stackKey: stackKey}
	for _, name := range slices.Sorted(maps.Keys(trees)) {
		w.walk(trees[name].Root, map[string]int{})
	}
	return w.reads
}

type configWalker struct {
	stackKey string
	reads    []configRead
}

// walk records the config reads under n. vars holds, per variable in scope, the prefix length of
// the config it is bound to, 0 for none; a block's declarations go into a copy.
func (w *configWalker) walk(n parse.Node, vars map[string]int) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			w.walk(c, vars)
		}
	case *parse.ActionNode:
		w.walk(n.Pipe, vars)
	case *parse.TemplateNode:
		w.walk(n.Pipe, vars)
	case *parse.IfNode:
		w.branch(&n.BranchNode, vars, false)
	case *parse.WithNode:
		w.branch(&n.BranchNode, vars, false)
	case *parse.RangeNode:
		w.branch(&n.BranchNode, vars, true)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, cmd := range n.Cmds {
			for _, arg := range cmd.Args {
				w.walk(arg, vars)
			}
		}
		bound := 0
		if len(n.Cmds) == 1 && len(n.Cmds[0].Args) == 1 {
			bound = w.config(n.Cmds[0].Args[0])
		}
		for _, v := range n.Decl {
			vars[v.Ident[0]] = bound
		}
	case *parse.ChainNode:
		w.walk(n.Node, vars)
	case *parse.FieldNode:
		w.read(n.Pos, n.Ident, 1)
	case *parse.VariableNode:
		switch {
		case n.Ident[0] == "$":
			w.read(n.Pos, n.Ident[1:], 0)
		case vars[n.Ident[0]] > 0 && len(n.Ident) > 1:
			w.reads = append(w.reads, configRead{pos: int(n.Pos) + 1, key: n.Ident[1], dot: vars[n.Ident[0]] == 1})
		}
	}
}

// branch walks an if, with or range. Range declarations bind elements, never the config.
func (w *configWalker) branch(n *parse.BranchNode, vars map[string]int, isRange bool) {
	inner := maps.Clone(vars)
	w.walk(n.Pipe, inner)
	if isRange {
		for _, v := range n.Pipe.Decl {
			inner[v.Ident[0]] = 0
		}
	}
	w.walk(n.List, inner)
	w.walk(n.ElseList, inner)
}

// config returns the prefix length when n is the config, from `.` or `$`, else 0.
func (w *configWalker) config(n parse.Node) int {
	switch n := n.(type) {
	case *parse.FieldNode:
		return w.configChain(n.Ident)
	case *parse.VariableNode:
		if n.Ident[0] == "$" {
			return w.configChain(n.Ident[1:])
		}
	}
	return 0
}

func (w *configWalker) configChain(ident []string) int {
	if n := w.prefix(ident); n == len(ident) {
		return n
	}
	return 0
}

// prefix returns how many links at the start of ident name the config: 1 for Config, 4 for
// Values.stack.<stackKey>.config, else 0.
func (w *configWalker) prefix(ident []string) int {
	switch {
	case len(ident) > 0 && ident[0] == "Config":
		return 1
	case len(ident) >= 4 && slices.Equal(ident[:4], []string{"Values", "stack", w.stackKey, "config"}):
		return 4
	}
	return 0
}

// read records the key a config chain reads. pos is where ident[from]'s dot starts: parse positions
// a chain at its second link.
func (w *configWalker) read(pos parse.Pos, ident []string, from int) {
	n := w.prefix(ident)
	if n == 0 || len(ident) <= n {
		return
	}
	p := int(pos)
	for _, id := range ident[from:n] {
		p += len(id) + 1
	}
	w.reads = append(w.reads, configRead{pos: p + 1, key: ident[n], dot: n == 1})
}
