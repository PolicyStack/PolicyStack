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
)

// ConfigReadCheck (POLICY022) reports a converter reading a top-level config key the element's
// values.yaml does not define: Helm renders it as null without complaint. It recognises
// `.Values.stack.<el>.config.<key>`, the same from `$`, and `$v.<key>` where `$v` is bound to
// `.Values.stack.<el>.config` in scope. Only Helm actions are read, so an escaped ACM template, text
// to Helm, is skipped. Each lifecycle component checks reads of its own stack key.
type ConfigReadCheck struct{}

func (ConfigReadCheck) ID() string   { return "POLICY022" }
func (ConfigReadCheck) Phase() Phase { return PhaseChart }

func (c *ConfigReadCheck) Run(ctx Context) []Finding {
	el := ctx.Element
	if el == nil || el.StackKey == "" {
		return nil
	}
	keys := configKeys(el)
	var out []Finding
	_ = filepath.WalkDir(el.ConvertersDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(data)
		for _, r := range configReadsIn(path, text, el.StackKey) {
			if slices.Contains(keys, r.key) {
				continue
			}
			before := text[:r.pos]
			out = append(out, Finding{
				RuleID:   c.ID(),
				Severity: SevError,
				Element:  el.ChartName,
				Message:  fmt.Sprintf("reads stack.%s.config.%s, which is not in the element's values.yaml", el.StackKey, r.key),
				File:     path,
				Line:     1 + strings.Count(before, "\n"),
				Col:      r.pos - strings.LastIndex(before, "\n"),
			})
		}
		return nil
	})
	return out
}

// configRead is a read of a top-level config key, pos its byte offset in the converter.
type configRead struct {
	pos int
	key string
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
		w.walk(trees[name].Root, map[string]bool{})
	}
	return w.reads
}

type configWalker struct {
	stackKey string
	reads    []configRead
}

// walk records the config reads under n. vars holds, per variable in scope, whether it is bound
// to the config; a block's declarations go into a copy.
func (w *configWalker) walk(n parse.Node, vars map[string]bool) {
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
		bound := len(n.Cmds) == 1 && len(n.Cmds[0].Args) == 1 && w.isConfig(n.Cmds[0].Args[0])
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
		case vars[n.Ident[0]] && len(n.Ident) > 1:
			w.reads = append(w.reads, configRead{pos: int(n.Pos) + 1, key: n.Ident[1]})
		}
	}
}

// branch walks an if, with or range. Range declarations bind elements, never the config.
func (w *configWalker) branch(n *parse.BranchNode, vars map[string]bool, isRange bool) {
	inner := maps.Clone(vars)
	w.walk(n.Pipe, inner)
	if isRange {
		for _, v := range n.Pipe.Decl {
			inner[v.Ident[0]] = false
		}
	}
	w.walk(n.List, inner)
	w.walk(n.ElseList, inner)
}

// isConfig reports whether n is `.Values.stack.<stackKey>.config` or the same from `$`.
func (w *configWalker) isConfig(n parse.Node) bool {
	switch n := n.(type) {
	case *parse.FieldNode:
		return w.configChain(n.Ident)
	case *parse.VariableNode:
		return n.Ident[0] == "$" && w.configChain(n.Ident[1:])
	}
	return false
}

func (w *configWalker) configChain(ident []string) bool {
	return slices.Equal(ident, []string{"Values", "stack", w.stackKey, "config"})
}

// read records the key a Values.stack.<stackKey>.config.<key> chain reads. pos is where
// ident[from]'s dot starts: parse positions a chain at its second link.
func (w *configWalker) read(pos parse.Pos, ident []string, from int) {
	if len(ident) < 5 || !w.configChain(ident[:4]) {
		return
	}
	p := int(pos)
	for _, id := range ident[from:4] {
		p += len(id) + 1
	}
	w.reads = append(w.reads, configRead{pos: p + 1, key: ident[4]})
}
