#!/usr/bin/env python3
# /// script
# requires-python = ">=3.10"
# dependencies = ["ruamel.yaml>=0.18"]
# ///
"""Migrate PolicyStack elements and values layers from policy-library 3.x to 4.0.

Usage: tools/migrate-4.0.py [path ...]    (default: the repository)

Elements (charts that depend on policy-library):
  - converters move to converters/<policy>/<template>.yaml, so ConfigurationPolicy names stay
    <policy>-<template>. Several templateNames are joined with ---, or become a raw list when their
    complianceType differs. A file several templates share moves to a _ partial named by file:.
  - policies and configPolicies become the policies.<p>.templates.<t> maps, and keys equal to a 4.0
    default are dropped. Actions and severities resolve so every object keeps its effective action.
  - .Parameters and .Values.stack.<element>.config become .Config, or a literal when every read of
    the key is a plain {{ .Parameters.x }}. A key several templates read alike moves to element
    config. The ns-monitoring converter becomes clusterMonitoring: true.
  - toggles become enabled: false, and the Chart.yaml pin moves to 4.0.0.
Values layers: toggles become policies.<p>.enabled, policies.<p>.templates.<t>.enabled,
operatorPolicies.<op>.enabled or operatorPolicies.<op>.clusterMonitoring, resolved against the element.

Values are read the way Helm reads them (YAML 1.1: yes/on/off are bools). Stops before writing
anything if a migrated map uses a YAML merge key (<<).
Idempotent. Prints what it could not convert; review the result with git diff and Render Diff.
"""

import collections
import copy
import re
import subprocess
import sys
from pathlib import Path

from ruamel.yaml import YAML
from ruamel.yaml.comments import CommentedMap as Map
from ruamel.yaml.comments import CommentedSeq as Seq
from ruamel.yaml.comments import merge_attrib
from ruamel.yaml.error import CommentMark
from ruamel.yaml.scalarint import ScalarInt
from ruamel.yaml.tokens import CommentToken

LIBRARY = "4.0.0"
ENTRY_KEYS = (
    "complianceType",
    "metadataComplianceType",
    "recordDiff",
    "recreateOption",
    "objectSelector",
)
NS_MONITORING = re.compile(
    r'apiVersion: v1\nkind: Namespace\nmetadata:\n  name: (.+)\n  labels:\n    openshift.io/cluster-monitoring: "true"'
)
PLAIN = re.compile(r"\{\{ \$?\.Parameters\.(\w+) \}\}")
# plain scalars YAML 1.1 (Helm) reads as bools and ruamel (YAML 1.2) as strings
BOOL11 = {
    w: on
    for on, words in ((True, "y yes on"), (False, "n no off"))
    for x in words.split()
    for w in (x, x.title(), x.upper())
}
V3_LAYER_KEYS = {
    "configPolicies",
    "templateParameters",
    "policyRef",
    "templateNames",
    "orderManifests",
    "orderPolicies",
}
notes = []


def note(where, msg):
    notes.append(f"{where}: {msg}")


def git(*args, cwd):
    return subprocess.run(
        ["git", *args], check=False, cwd=cwd, capture_output=True, text=True
    )


def tracked(path):
    return (
        git("ls-files", "--error-unmatch", path.name, cwd=path.parent).returncode == 0
    )


class Doc:
    """A YAML file edited in place. Comments and blank lines are re-attached by source line on save,
    so a key the migrator moves keeps the block above it."""

    def __init__(self, path):
        self.path = path
        self.text = path.read_text()
        self.lines = self.text.split("\n")
        m = re.search(
            r"^( *)\S[^\n]*:\n(?:\1 +#[^\n]*\n)*( *)- ", self.text, re.MULTILINE
        )
        self.offset = len(m[2]) - len(m[1]) if m and len(m[2]) > len(m[1]) else 2
        self.yaml = YAML()
        self.yaml.preserve_quotes = True
        self.yaml.width = 4096
        self.yaml.indent(mapping=2, sequence=self.offset + 2, offset=self.offset)
        if re.search(r":\s+null\s*$", self.text, re.MULTILINE):
            self.yaml.representer.add_representer(
                type(None),
                lambda r, _: r.represent_scalar("tag:yaml.org,2002:null", "null"),
            )
        self.data = self.yaml.load(self.text)
        self.comments = set()
        if self.data is not None:
            self._collect(self.data)
        self.placed = {}
        self.taken = set()  # comment lines the migrator moved elsewhere
        self.eols = []  # (map, key, comment) for the key a toggle became

    def _collect(self, node):
        for slot in [node.ca.comment, node.ca.end, *node.ca.items.values()]:
            todo = [slot]
            while todo:
                t = todo.pop()
                if isinstance(t, list):
                    todo += t
                elif isinstance(t, CommentToken):
                    for i, _ in enumerate(t.value.rstrip("\n").split("\n")):
                        line = t.start_mark.line + i
                        if line < len(self.lines) and self.lines[
                            line
                        ].lstrip().startswith("#"):
                            self.comments.add(line)
        for v in (
            node.values()
            if isinstance(node, Map)
            else node
            if isinstance(node, Seq)
            else ()
        ):
            if isinstance(v, (Map, Seq)):
                self._collect(v)

    def place(self, node, key, *lines):
        """Give a key the comment blocks above source lines `lines`, in order."""
        self.placed.setdefault(id(node), {})[key] = [x for x in lines if x is not None]

    def line(self, node, key):
        if key in self.placed.get(id(node), {}):
            return self.placed[id(node)][key]
        lc = (node.lc.data or {}).get(key)
        return [lc[0]] if lc else []

    def save(self):
        self._strip(self.data, set())
        for node, key, eol in self.eols:
            node.yaml_add_eol_comment(eol, key)
        used = set()

        def block(line, raw=False):
            out, i = [], line - 1
            while (
                i >= 0
                and i not in used
                and (not self.lines[i].strip() or i in self.comments)
            ):
                out.insert(0, self.lines[i].rstrip() if raw else self.lines[i].strip())
                used.add(i)
                i -= 1
            return out

        def walk(node, col, seen):
            if id(node) in seen:
                return
            seen.add(id(node))
            items = (
                list(node.items()) if isinstance(node, Map) else list(enumerate(node))
            )
            for i, (k, v) in enumerate(items):
                lead, pre = [], []
                for blk in (block(line) for line in self.line(node, k)):
                    n = next((j for j, x in enumerate(blk) if x), len(blk))
                    lead, pre = lead or blk[:n], pre + blk[n:]
                # a key the migrator placed first in its map starts right below its parent
                pre = (
                    pre if i == 0 and k in self.placed.get(id(node), {}) else lead + pre
                )
                if pre:
                    slot = node.ca.items.setdefault(k, [None, None, None, None])
                    slot[1] = [CommentToken(s + "\n", CommentMark(col)) for s in pre]
                if isinstance(v, (Map, Seq)):
                    walk(
                        v,
                        col
                        + (
                            self.offset
                            if isinstance(node, Map) and isinstance(v, Seq)
                            else 2
                        ),
                        seen,
                    )

        walk(self.data, 0, set())
        tail = block(len(self.lines), raw=True)
        while tail and not tail[-1]:
            tail.pop()
        if any(tail):
            node, key = self.data, None
            while isinstance(node, (Map, Seq)) and len(node):
                parent, key = (
                    node,
                    list(node)[-1] if isinstance(node, Map) else len(node) - 1,
                )
                node = node[key]
            slot = parent.ca.items.setdefault(key, [None, None, None, None])
            i = 2 if isinstance(parent, Map) else 0
            eol = slot[i].value.rstrip("\n") if slot[i] else ""
            slot[i] = CommentToken(eol + "\n" + "\n".join(tail) + "\n", CommentMark(0))
        for line in sorted(self.comments - used - self.taken):
            note(
                self.path,
                f"line {line + 1}: comment dropped: {self.lines[line].strip()}",
            )
        with self.path.open("w") as f:
            self.yaml.dump(self.data, f)

    def _strip(self, node, seen):
        """Drop every full-line comment, keeping end-of-line ones; save() puts the rest back."""
        if id(node) in seen:
            return
        seen.add(id(node))
        node.ca.comment = None
        node.ca.end = []
        for k in list(node.ca.items):
            lc = (node.lc.data or {}).get(k)
            lines = {lc[0], lc[2] if len(lc) > 2 else lc[0]} if lc else set()
            i = 2 if isinstance(node, Map) else 0
            t = node.ca.items[k][i] if len(node.ca.items[k]) > i else None
            keep = None
            if (
                isinstance(t, CommentToken)
                and t.start_mark.line in lines
                and t.value.lstrip(" ").startswith("#")
            ):
                keep = (
                    t
                    if t.value.count("\n") <= 1
                    else CommentToken(t.value.split("\n")[0] + "\n", t.start_mark)
                )
            node.ca.items[k] = [None, None, None, None]
            node.ca.items[k][i] = keep
        for v in node.values() if isinstance(node, Map) else node:
            if isinstance(v, (Map, Seq)):
                self._strip(v, seen)


def helm(v):
    """Go template truthiness of a value as Helm reads it."""
    return BOOL11[v] if type(v) is str and v in BOOL11 else bool(v)


def put(m, key, value, after=None):
    """Set m[key], inserting a new key after `after` (or at the end)."""
    if key in m:
        m[key] = value
    elif after in m:
        m.insert(list(m).index(after) + 1, key, value)
    else:
        m[key] = value


def set_enabled(m, on):
    """4.0 entries are on unless enabled: false."""
    if on:
        m.pop("enabled", None)
    elif "enabled" in m:
        m["enabled"] = False
    else:
        m.insert(0, "enabled", False)


def mode(values, fallback):
    values = [v for v in values if v]
    return collections.Counter(values).most_common(1)[0][0] if values else fallback


def is_v3(comp):
    return isinstance(comp, Map) and (
        isinstance(comp.get("policies"), list)
        or "configPolicies" in comp
        or "toggles" in comp
    )


def model(chart, comp):
    """Policies, templates (name -> policies) and operators of a 3.x or 4.0 component. migrate fills
    monitoring: ns-monitoring templates it replaced, by owning operator."""
    ops = set((comp.get("operatorPolicies") or {}).keys())
    templates = collections.defaultdict(list)
    if isinstance(comp.get("policies"), list):
        pols = {p["name"] for p in comp["policies"]}
        for c in comp.get("configPolicies") or []:
            templates[c["name"]].append(c["policyRef"])
    else:
        pols = set((comp.get("policies") or {}).keys())
        for f in (chart / "converters").glob("[!_]*/[!_]*.yaml"):
            pols.add(f.parent.name)
            templates[f.stem].append(f.parent.name)
        for p, pv in (comp.get("policies") or {}).items():
            for t in (pv or {}).get("templates") or {}:
                if p not in templates[t]:
                    templates[t].append(p)
        if ops:
            pols.add("install")
    return {
        "policies": pols,
        "templates": templates,
        "operators": ops,
        "monitoring": {},
    }


class Element:
    def __init__(self, chart):
        self.chart = chart
        self.conv = chart / "converters"
        self.doc = Doc(chart / "values.yaml")
        self.stack = (self.doc.data or {}).get("stack") or Map()
        self.multi = len(self.stack) > 1
        self.where = chart / "values.yaml"
        self.files = {}  # target -> text
        self.sources = {}  # target -> source paths it came from, first one is git mv'd
        self.users = collections.defaultdict(list)  # target -> [(template map, params)]
        self.dropped = set()  # sources nothing uses any more
        self.templates = []  # (template map, params, component)
        self.toggled = {}  # id(entry) -> source line of the toggle that set it
        self.refs = {}  # converter -> policies using it
        self.models = {
            name: model(chart, comp)
            for name, comp in self.stack.items()
            if isinstance(comp, Map)
        }

    def migrate(self):
        names = [n for n, c in self.stack.items() if is_v3(c)]
        for name in names:
            self.component(name, self.stack[name])
        self.parameters()
        self.write()
        for name, comp in self.stack.items():
            if isinstance(comp, Map) and not self.multi:
                self.config_refs(name)
        if names:
            self.doc.save()
        self.pin()

    # values ---------------------------------------------------------------------------------

    def component(self, name, comp):
        where = f"{self.where} stack.{name}"
        doc = self.doc
        policies = comp.get("policies") or Seq()
        cps = list(comp.get("configPolicies") or [])
        ops = comp.get("operatorPolicies") or Map()
        pol = {}
        for p in policies:
            if p["name"] in pol:
                note(where, f"policy {p['name']} is declared twice; the first wins")
                continue
            pol[p["name"]] = p
        for c in cps:
            if c.get("policyRef") not in pol:
                note(
                    where,
                    f"configPolicy {c.get('name')} has policyRef {c.get('policyRef')!r}, which names no policy (3.x never rendered it); dropped, its converters stay",
                )
        cps = [c for c in cps if c.get("policyRef") in pol]

        # toggles override enabled
        toggles = comp.get("toggles") or {}
        eols = []  # (toggle line, hits, end-of-line comment)
        for t, v in toggles.items():
            if v is None:
                continue
            v = helm(v)
            # a false toggle on a policy already hides its same-named template
            hits = ([pol[t]] if t in pol else []) + [
                c for c in cps if c["name"] == t and (v or c["policyRef"] != t)
            ]
            hits += [ops[t]] if t in ops else []
            if not hits:
                note(
                    where,
                    f"toggle {t} matches no policy, configPolicy or operator; dropped",
                )
            for h in hits:
                if "enabled" in h:
                    h["enabled"] = v
                else:
                    h.insert(0, "enabled", v)
                self.toggled[id(h)] = toggles.lc.key(t)[0]
            if eol := eol_comment(toggles, t):
                eols.append((toggles.lc.key(t)[0], hits, eol))

        def enabled(e):  # 3.x: on when enabled is truthy
            return helm(e.get("enabled"))

        # ns-monitoring converters become clusterMonitoring on the operator owning the namespace
        for c in list(cps):
            files = [
                t if isinstance(t, str) else t["name"]
                for t in c.get("templateNames") or []
            ]
            src = self.conv / f"{files[0]}.yaml" if len(files) == 1 else None
            m = (
                NS_MONITORING.fullmatch(src.read_text().strip())
                if src and src.exists()
                else None
            )
            if not m or c.get("complianceType", "musthave") != "musthave":
                continue
            ns = m[1]
            if pm := PLAIN.fullmatch(ns):
                ns = (
                    (c.get("templateParameters") or {}).get(pm[1])
                    if helm(c.get("enableTemplateParameters"))
                    else None
                )
            owner = [
                o
                for o, ov in ops.items()
                if ov.get("namespace") == ns
                and (ov.get("policyRef") or "install") == c["policyRef"]
            ]
            if (
                owner
                and enabled(c)
                and enabled(ops[owner[0]])
                and c["policyRef"] == "install"
            ):
                put(ops[owner[0]], "clusterMonitoring", True, after="namespace")
                self.models[name]["monitoring"][c["name"]] = owner[0]
                cps.remove(c)
                self.dropped.add(src)
            else:
                note(
                    where,
                    f"configPolicy {c['name']} labels namespace {ns} for cluster monitoring but no enabled install operator owns it; kept as a converter",
                )

        # actions and severities: the 4.0 Policy action applies to every non-InformOnly template
        action, severity = {}, {}
        explicit = {p: bool(pv.get("remediationAction")) for p, pv in pol.items()}
        for p, pv in pol.items():
            mine = [c for c in cps if c["policyRef"] == p]
            mine_ops = [
                o for o in ops.values() if (o.get("policyRef") or "install") == p
            ]
            if pv.get("remediationAction"):
                action[p] = str(pv["remediationAction"]).lower()
            else:
                acts = {
                    str(c.get("remediationAction") or "inform").lower() for c in mine
                } | {
                    str(o["remediationAction"]).lower()
                    for o in mine_ops
                    if o.get("remediationAction")
                }
                action[p] = "enforce" if "enforce" in acts else "inform"
            severity[p] = pv.get("severity") or mode(
                [c.get("severity") or "low" for c in mine], "low"
            )
        default = comp.get("default")
        default_action = mode(list(action.values()), "inform")
        default_severity = mode(list(severity.values()), "low")
        for key, value, fallback in (
            ("remediationAction", default_action, "inform"),
            ("severity", default_severity, "low"),
        ):
            old = (default or {}).get(key)
            if old is not None and old != value:
                note(
                    where,
                    f"default.{key} {old!r} was ignored by 3.x and is now {value!r}",
                )
            if value != fallback:
                if default is None:
                    default = Map()
                    comp.insert(
                        list(comp).index("enabled") + 1 if "enabled" in comp else 0,
                        "default",
                        default,
                    )
                if key in default:
                    default[key] = value
                else:
                    at = int(key == "severity" and "remediationAction" in default)
                    default.insert(at, key, value)
            elif default is not None:
                default.pop(key, None)
        default_severity = (default or {}).get("severity") or "low"
        default_action = (default or {}).get("remediationAction") or "inform"

        # operators
        if ops and "install" not in pol:
            note(
                where,
                "operatorPolicies render into policy install in 4.0, which is not declared",
            )
        for o, ov in ops.items():
            ref = ov.pop("policyRef", None) or "install"
            if ref != "install":
                note(
                    where,
                    f"operator {o} had policyRef {ref}; 4.0 renders it into install, renaming its objects to install-{o}",
                )
            set_enabled(ov, enabled(ov))
            policy_action = (
                str(pol[ref].get("remediationAction") or "").lower()
                if ref in pol
                else ""
            )
            op_action = policy_action or str(ov.get("remediationAction") or "").lower()
            if ov.pop(
                "remediationAction", None
            ) is not None and op_action != action.get("install", default_action):
                note(
                    where,
                    f"operator {o} acted {op_action}, now {action.get('install', default_action)} (the install Policy's action)",
                )
            op_sev = ov.get("severity") or (
                pol[ref].get("severity") if ref in pol else None
            )
            if op_sev is None:
                note(
                    where, f"operator {o} rendered no severity in 3.x; it now inherits"
                )
            if op_sev in (None, severity.get("install", default_severity)):
                ov.pop("severity", None)
            if ov.get("complianceType") == "musthave":
                del ov["complianceType"]
            if ov.get("upgradeApproval") == "Automatic":
                del ov["upgradeApproval"]
            sub = ov.get("subscription") or {}
            for key, value in (
                ("source", "redhat-operators"),
                ("sourceNamespace", "openshift-marketplace"),
            ):
                if sub.get(key) == value:
                    del sub[key]
            og = ov.get("operatorGroup")
            if og and og.get("name") == sub.get("name"):
                del og["name"]
                if not og:
                    del ov["operatorGroup"]

        # converter layout
        uses = collections.Counter()
        self.refs = collections.defaultdict(set)  # file -> policies using it
        for comp2 in self.stack.values():
            for c in (
                (comp2.get("configPolicies") or []) if isinstance(comp2, Map) else []
            ):
                for t in c.get("templateNames") or []:
                    uses[t if isinstance(t, str) else t["name"]] += 1
                    self.refs[t if isinstance(t, str) else t["name"]].add(
                        c["policyRef"]
                    )
        mine = {p: [c for c in cps if c["policyRef"] == p] for p in pol}
        for (p, t), n in collections.Counter(
            (c["policyRef"], c["name"]) for c in cps
        ).items():
            if n > 1:
                note(
                    where,
                    f"configPolicy {p}/{t} is declared {n} times; one ConfigurationPolicy name",
                )

        order_all = helm(comp.get("orderManifests"))
        out = Map()
        for p, pv in pol.items():
            doc.place(out, p, self.toggled.get(id(pv)), seq_line(policies, pv))
            out[p] = pv
            del pv["name"]
            pv.pop("namespace", None)
            set_enabled(pv, enabled(pv))
            if pv.get("disabled") is False:
                del pv["disabled"]
            pv.pop("remediationAction", None)
            if action[p] != default_action:
                put(pv, "remediationAction", action[p], after="description")
            if severity[p] != default_severity:
                put(
                    pv,
                    "severity",
                    severity[p],
                    after="remediationAction"
                    if "remediationAction" in pv
                    else "description",
                )
            else:
                pv.pop("severity", None)
            order = helm(pv.pop("orderManifests", order_all))
            templates = Map()
            for c in mine[p]:
                t = c["name"]
                doc.place(
                    templates,
                    t,
                    self.toggled.get(id(c)),
                    seq_line(comp["configPolicies"], c),
                )
                templates[t] = self.template(
                    where, comp, c, p, uses, explicit[p], action[p], severity[p]
                )
            if order and templates:
                pv["orderManifests"] = Seq(templates)
                pv["orderManifests"].fa.set_flow_style()
            elif order:
                note(
                    where,
                    f"policy {p} chained its operators with orderManifests, which 4.0 cannot express without templates",
                )
            if templates:
                pv["templates"] = templates
                line = comp.lc.key("configPolicies")[0]
                if (
                    len([x for x in mine.values() if x]) == 1
                    and line - 1 in doc.comments
                ):
                    doc.place(pv, "templates", line)
        for o in ops:
            doc.place(ops, o, self.toggled.get(id(ops[o])), ops.lc.key(o)[0])
        live = {id(x) for x in (*pol.values(), *cps, *ops.values())}
        for line, hits, eol in eols:
            keep = next((h for h in hits if id(h) in live and "enabled" in h), None)
            if keep is None:
                note(self.where, f"line {line + 1}: comment dropped: {eol}")
            else:
                doc.eols.append((keep, "enabled", eol))
        if "policies" in comp:
            comp["policies"] = out
        else:
            comp.insert(len(comp), "policies", out)
        doc.place(
            comp,
            "policies",
            *[comp.lc.key(k)[0] for k in ("policies", "toggles") if k in comp.lc.data],
        )
        for key in ("configPolicies", "toggles", "orderManifests"):
            comp.pop(key, None)
        if comp.pop("orderPolicies", None):
            note(
                where,
                "orderPolicies is removed in 4.0; chain the policies with dependencies",
            )
        if comp.get("certificatePolicies"):
            note(
                where,
                "certificatePolicies is removed in 4.0; left in place, convert by hand",
            )
        elif "certificatePolicies" in comp:
            del comp["certificatePolicies"]

    def template(
        self, where, comp, c, p, uses, explicit, policy_action, policy_severity
    ):
        t = c["name"]
        entries = [
            Map(name=e) if isinstance(e, str) else e
            for e in c.get("templateNames") or []
        ]
        files = [e["name"] for e in entries]
        on = helm(c.get("enableTemplateParameters"))
        params = c.get("templateParameters") if on else None
        if c.get("templateParameters") and not on:
            note(
                where,
                f"{p}/{t}: templateParameters without enableTemplateParameters were never passed; dropped",
            )
        raw = helm(c.get("rawTemplate"))
        self.templates.append((c, params, comp))

        # per-object keys: hoisted when uniform, otherwise the template becomes a raw list
        per = []
        for e in entries:
            keys = {k: e[k] for k in ENTRY_KEYS if k in e}
            if c.get("complianceType"):
                keys["complianceType"] = c["complianceType"]
            keys.setdefault("complianceType", "musthave")
            per.append(keys)
        to_raw = not raw and len({repr(sorted(k.items())) for k in per}) > 1
        if not raw and not to_raw and per:
            for k, v in per[0].items():
                put(c, k, v, after="description")

        # layout
        file_key = None
        if self.multi:
            targets = [f if "/" not in f else "_" + f.lstrip("_") for f in files]
            for f, target in zip(files, targets):
                self.add(target, [f], c, params)
            file_key = targets
        elif len(files) == 1 and uses[files[0]] > 1:
            target = self.partial(files[0])
            self.add(target, files, c, params)
            file_key = [target]
        elif any(uses[f] > 1 for f in files):
            if to_raw:
                note(
                    where,
                    f"{p}/{t}: per-object complianceType with a shared file; converted without it, fix by hand",
                )
            file_key, chunk = [], []
            for f in files + [None]:
                if f is not None and uses[f] == 1:
                    chunk.append(f)
                    continue
                if chunk:
                    target = (
                        f"{p}/{t}"
                        if f"{p}/{t}" not in file_key
                        else f"{p}/_{t}-{len(file_key)}"
                    )
                    self.add(target, chunk, c, params)
                    file_key.append(target)
                    chunk = []
                if f is not None:
                    target = self.partial(f)
                    self.add(target, [f], c, params)
                    file_key.append(target)
        else:
            comments = []
            for i in range(len(entries) if to_raw else 0):
                line, block = (
                    seq_line(c["templateNames"], c["templateNames"][i]) - 1,
                    [],
                )
                while line in self.doc.comments:
                    block.insert(0, self.doc.lines[line].strip().lstrip("#").strip())
                    self.doc.taken.add(line)
                    line -= 1
                comments.append(block)
            self.add(f"{p}/{t}", files, c, params, (per, comments) if to_raw else None)

        # keys
        for key in (
            "name",
            "policyRef",
            "rawTemplate",
            "templateNames",
            "enableTemplateParameters",
            "templateParameters",
        ):
            c.pop(key, None)
        set_enabled(c, helm(c.get("enabled")))
        if c.get("complianceType") == "musthave" or to_raw or raw:
            c.pop("complianceType", None)  # a raw list sets it per object
        if file_key:
            put(
                c,
                "file",
                file_key[0] if len(file_key) == 1 else Seq(file_key),
                after="description",
            )
        action = str(c.get("remediationAction") or "inform")
        # a 3.x Policy action overrode the template's; without one, an inform template kept its own
        if action == "InformOnly" or (
            not explicit and action.lower() == "inform" and policy_action == "enforce"
        ):
            c["remediationAction"] = "InformOnly"
        else:
            c.pop("remediationAction", None)
        sev = c.get("severity") or "low"
        if sev == policy_severity:
            c.pop("severity", None)
        else:
            put(
                c,
                "severity",
                sev,
                after="remediationAction"
                if "remediationAction" in c
                else "description",
            )
        if c.get("waitForOperator"):
            if helm(c.get("ignorePending")):
                del c["ignorePending"]
            else:
                put(c, "ignorePending", False, after="waitForOperator")
        elif "ignorePending" in c and not helm(c["ignorePending"]):
            del c["ignorePending"]
        return c

    # converters ---------------------------------------------------------------------------

    def partial(self, f):
        """A shared file's partial: <policy>/_<file> when one policy uses it, else _shared/<file>."""
        refs = self.refs[f]
        base = f.rsplit("/", 1)[-1]
        return f"{next(iter(refs))}/_{base}" if len(refs) == 1 else f"_shared/{base}"

    def add(self, target, files, c, params, raw=None):
        self.users[target].append((c, params))
        if target in self.files:
            return
        texts = []
        for f in files:
            src = self.conv / f"{f}.yaml"
            if not src.exists():
                note(self.where, f"converter {src} is missing")
                texts.append("")
                continue
            texts.append(src.read_text())
        self.sources[target] = [self.conv / f"{f}.yaml" for f in files]
        if raw:
            self.files[target] = raw_list(texts, *raw)
        elif len(texts) == 1:
            self.files[target] = texts[0]
        else:
            # Helm-only lines next to --- lose their right trim, which would glue --- to a document
            last = len(texts) - 1
            self.files[target] = (
                "\n---\n".join(untrim(x, i > 0, i < last) for i, x in enumerate(texts))
                + "\n"
            )

    def parameters(self):
        """.Parameters -> .Config. A key every converter reads as a plain {{ .Parameters.x }} becomes a
        literal where all users agree. Otherwise a key several templates of a component read and agree
        on moves to element config, else to templates.<t>.config."""
        helpers = {}
        for tpl in (self.chart / "templates").glob("*.tpl"):
            text = tpl.read_text()
            for m in re.finditer(
                r'define "([^"]+)"(.*?)\{\{-? end -?\}\}', text, re.DOTALL
            ):
                if keys := set(re.findall(r"\.Parameters\.(\w+)", m[2])):
                    helpers[m[1]] = keys
            if ".Parameters" in text and not self.multi:
                tpl.write_text(
                    text.replace(".Parameters.", ".Config.").replace(
                        ".Parameters", ".Config"
                    )
                )
        comp_of = {id(c): comp for c, _, comp in self.templates}
        reads = {}  # target -> keys it reads other than as a plain {{ .Parameters.x }}
        expr = collections.defaultdict(set)  # id(component) -> such keys
        readers = collections.defaultdict(
            dict
        )  # (id(component), key) -> {id(c): params}
        for target, text in self.files.items():
            users = self.users[target]
            keys = set(re.findall(r"\.Parameters\.(\w+)", PLAIN.sub("", text)))
            for h, hkeys in helpers.items():
                if f'include "{h}"' in text:
                    keys |= hkeys
            if re.search(
                r"\.Parameters\b(?!\.)",
                re.sub(r"\{\{-? */\*.*?\*/ *-?\}\}", "", text, flags=re.DOTALL),
            ):
                note(
                    self.where,
                    f"converters/{target}.yaml reads .Parameters whole; now .Config, check it",
                )
                keys |= {k for _, params in users for k in params or {}}
            reads[target] = keys
            for c, params in users:
                comp = comp_of[id(c)]
                expr[id(comp)] |= keys
                for k in keys | set(PLAIN.findall(text)):
                    if k in (params or {}):
                        readers[id(comp), k][id(c)] = params
        lifted = set()  # (id(component), key) moved to element config
        for _, params, comp in self.templates:
            for k in params or {}:
                values = list(readers[id(comp), k].values())
                cfg = comp.get("config")
                if (
                    (id(comp), k) in lifted
                    or k not in expr[id(comp)]
                    or len(values) < 2
                    or any(p[k] != values[0][k] for p in values)
                    or k in (cfg or {})
                ):
                    continue
                if cfg is None:
                    cfg = Map()
                    put(
                        comp,
                        "config",
                        cfg,
                        "default" if "default" in comp else "enabled",
                    )
                cfg[k] = copy.deepcopy(values[0][k])
                self.doc.place(cfg, k, values[0].lc.key(k)[0])
                lifted.add((id(comp), k))
        need = collections.defaultdict(set)  # id(template) -> keys
        for target, text in self.files.items():
            users = self.users[target]

            def inline(m, users=users):
                if any(m[1] in expr[id(comp_of[id(c)])] for c, _ in users):
                    return m[0]
                values = [(params or {}).get(m[1], KeyError) for _, params in users]
                lit = {literal(v) for v in values}
                return lit.pop() if len(lit) == 1 and None not in lit else m[0]

            text = PLAIN.sub(inline, text)
            keys = set(re.findall(r"\.Parameters\.(\w+)", text)) | reads[target]
            self.files[target] = text.replace(".Parameters.", ".Config.").replace(
                ".Parameters", ".Config"
            )
            for c, _ in users:
                need[id(c)] |= keys
        for c, params, comp in self.templates:
            params = params or {}
            keys = sorted(
                need[id(c)], key=lambda k: list(params).index(k) if k in params else -1
            )
            cfg, comp_cfg = Map(), comp.get("config") or {}
            desc = c.get("description", "")
            for k in keys:
                if k not in params:
                    note(
                        self.where,
                        f"a converter of template {desc!r} reads .Parameters.{k}, which the template never set",
                    )
                elif (id(comp), k) in lifted:
                    continue
                elif k in comp_cfg and comp_cfg[k] == params[k]:
                    note(
                        self.where,
                        f"template config {k} ({desc!r}) equals element config {k}, so it was dropped; layer overrides of config.{k} now reach its converters",
                    )
                else:
                    if k in comp_cfg:
                        note(
                            self.where,
                            f"template config {k} ({desc!r}) overrides element config {k}; check its converters",
                        )
                    cfg[k] = copy.deepcopy(params[k])
                    self.doc.place(cfg, k, params.lc.key(k)[0])
            if cfg:
                c["config"] = cfg

    def write(self):
        moved = set()
        for target, text in self.files.items():
            dst = self.conv / f"{target}.yaml"
            srcs = self.sources[target]
            first = next((s for s in srcs if s.exists() and s not in moved), None)
            if dst in srcs:
                pass
            elif dst.exists():
                note(self.where, f"{dst} already exists; not overwritten")
                continue
            elif first:
                dst.parent.mkdir(parents=True, exist_ok=True)
                if (
                    tracked(first)
                    and git("mv", str(first), str(dst), cwd=self.chart).returncode == 0
                ):
                    pass
                else:
                    first.rename(dst)
                moved.add(first)
            dst.write_text(text)
        used = {s for target in self.files for s in self.sources[target]}
        for src in (used - moved - {self.conv / f"{t}.yaml" for t in self.files}) | (
            self.dropped - used
        ):
            if src.exists():
                if tracked(src):
                    git("rm", "-q", str(src), cwd=self.chart)
                src.unlink(missing_ok=True)
        if (self.files or self.dropped) and not self.multi:
            for f in self.conv.rglob("*.yaml"):
                rel = f.relative_to(self.conv).with_suffix("").as_posix()
                if rel not in self.files and not re.search(r"(^|/)_", rel):
                    note(
                        self.where,
                        f"converters/{rel}.yaml is used by no template; move it under a _ directory or delete it",
                    )
        for d in sorted(self.conv.rglob("*"), reverse=True) + [self.conv]:
            if d.is_dir() and not any(d.iterdir()):
                d.rmdir()
        if self.multi and self.files:
            for tpl in (self.chart / "templates").glob("*"):
                if "converters/" in tpl.read_text():
                    note(
                        tpl,
                        "reads a converter path; engine directories now start with _",
                    )

    def config_refs(self, name):
        """.Values.stack.<element>.config and the chart-name index trick read .Config."""
        own = re.compile(rf"(\$?)\.Values\.stack\.{name}\.config\b")
        trick = re.compile(
            r"\(index (\$?)\.Values\.stack \([^()]*\.Chart\.Name[^()]*\)\)\.config\b"
        )
        for f in self.conv.rglob("*.yaml"):
            text = f.read_text()
            new = trick.sub(r"\1.Config", own.sub(r"\1.Config", text))
            if new != text:
                f.write_text(new)
            if re.search(rf"\.Values\.stack\.{name}\.toggles|\.toggles\b", new):
                note(
                    f,
                    "reads toggles; read policies.<p>.enabled (policy-library.enabled) instead",
                )

    def pin(self):
        chart = self.chart / "Chart.yaml"
        text = chart.read_text()
        new = re.sub(
            r"(- name: policy-library\n(?:\s+\w+:.*\n)*?\s+version:\s*)[\"']?3\.[^\"'\n]*[\"']?",
            rf'\g<1>"{LIBRARY}"',
            text,
        )
        if new != text:
            chart.write_text(new)


def eol_comment(m, key):
    """The end-of-line comment on m[key], or None."""
    t = (m.ca.items.get(key) or [None] * 3)[2]
    if isinstance(t, CommentToken) and t.value.lstrip(" ").startswith("#"):
        return t.value.split("\n")[0].strip()
    return None


def seq_line(seq, node):
    return seq.lc.item([id(x) for x in seq].index(id(node)))[0]


def literal(v):
    """v as converter text; None when Helm (YAML 1.1) may read it differently than ruamel (1.2)."""
    if isinstance(v, bool):
        return "true" if v else "false"
    # ruamel keeps 0644, 1_000 and 0x1F as ScalarInt, which Helm reads as other numbers
    if (
        type(v) is int
        or type(v) is ScalarInt
        and not v._underscore
        and v._width == len(str(v))
    ) and abs(v) < 10**6:
        return str(v)
    if type(v) is str and v in BOOL11:  # a plain yes renders true
        return None
    if isinstance(v, str) and "{{" not in v and "}}" not in v:
        return str(v)
    return None


ACTION = r"\{\{(?:[^}]|\}(?!\}))*\}\}"


def untrim(text, lead, tail):
    """Drop the right trim of the leading (lead) and trailing (tail) Helm-only lines."""
    head, body, foot = "", text.strip("\n"), ""

    def keep(m):  # a comment must end at the delimiter
        return "*/}}" if m[1] else " }}"

    while lead and (m := re.match(rf"[ \t]*{ACTION}[ \t]*\n", body)):
        head += re.sub(r"(\*/)?\s*-\}\}(?=[ \t]*\n$)", keep, m[0])
        body = body[m.end() :]
    while tail and (m := re.search(rf"\n[ \t]*{ACTION}[ \t]*$", body)):
        foot = re.sub(r"(\*/)?\s*-\}\}(?=[ \t]*$)", keep, m[0]) + foot
        body = body[: m.start()]
    return head + body + foot


def raw_list(texts, per, comments):
    """Objects with different complianceType as one object-templates-raw list."""
    head, body = [], []
    for text, keys, pre in zip(texts, per, comments):
        text = text.strip("\n")
        # Leading Helm-only lines go above the list, where their whitespace trims glue nothing
        while m := re.match(rf"{ACTION}[ \t]*(?:\n|$)", text):
            head.append(m[0].rstrip())
            text = text[m.end() :]
        text = untrim(text, False, True)
        body += [f"{{{{- /* {line} */}}}}" for line in pre]
        body.append(f"- complianceType: {keys['complianceType']}")
        body += [f"  {k}: {keys[k]}" for k in ENTRY_KEYS[1:] if k in keys]
        body.append("  objectDefinition:")
        for line in text.split("\n"):
            # a line starting with an indent action is already shifted by the raised indent
            shifted = line.lstrip().startswith("{{") and re.search(
                r"\bindent \d+", line
            )
            line = re.sub(
                r"\b(n?indent) (\d+)", lambda m: f"{m[1]} {int(m[2]) + 4}", line
            )
            body.append(line if shifted else f"    {line}" if line.strip() else "")
    return "\n".join(head + body) + "\n"


class Layer:
    """A values file: stack.<element>.toggles become enabled keys resolved against the element."""

    def __init__(self, path, models):
        self.path = path
        self.models = models
        self.doc = Doc(path)
        data = self.doc.data
        self.stack = (data.get("stack") if isinstance(data, Map) else None) or Map()

    def migrate(self):
        doc = self.doc
        changed = False
        for name, comp in self.stack.items():
            if not isinstance(comp, Map):
                continue
            for key in [k for k in comp if k in V3_LAYER_KEYS]:
                note(self.path, f"stack.{name}.{key} is 3.x; convert it by hand")
            if "toggles" not in comp:
                continue
            m = self.models.get(name)
            if m is None:
                note(
                    self.path,
                    f"stack.{name}: no element defines it; toggles left as they are",
                )
                continue
            if not all(
                isinstance(comp.get(k), (Map, type(None)))
                for k in ("policies", "operatorPolicies")
            ):
                note(
                    self.path,
                    f"stack.{name}: policies or operatorPolicies is not a map; toggles left as they are",
                )
                continue
            toggles = comp["toggles"] or Map()
            line = comp.lc.key("toggles")[0]
            pos = list(comp).index("toggles")
            new = {k: comp.get(k) is None for k in ("policies", "operatorPolicies")}
            policies = comp.get("policies") or Map()
            ops = comp.get("operatorPolicies") or Map()
            for t, v in toggles.items():
                if v is None:
                    continue
                v, at, owner = helm(v), toggles.lc.key(t)[0], m["monitoring"].get(t)
                hits = []
                if t in m["policies"]:
                    hits.append((policies, t, policies.setdefault(t, Map()), "enabled"))
                for p in [] if owner else m["templates"].get(t, []):
                    if p == t and not v:
                        continue  # a false toggle on a policy already hides its same-named template
                    entry = policies.setdefault(p, Map())
                    hits.append(
                        (
                            policies,
                            p,
                            entry.setdefault("templates", Map()).setdefault(t, Map()),
                            "enabled",
                        )
                    )
                if t in m["operators"]:
                    hits.append((ops, t, ops.setdefault(t, Map()), "enabled"))
                if owner:  # the element replaced this ns-monitoring template
                    hits.append(
                        (ops, owner, ops.setdefault(owner, Map()), "clusterMonitoring")
                    )
                if not hits:
                    note(
                        self.path,
                        f"stack.{name}.toggles.{t} matches nothing in the element; dropped",
                    )
                for i, (parent, key, entry, field) in enumerate(hits):
                    entry.pop(field, None)
                    entry.insert(0, field, v)
                    if i == 0:
                        doc.place(parent, key, at)
                        if eol := eol_comment(toggles, t):
                            doc.eols.append((entry, field, eol))
            del comp["toggles"]
            for key, value in (("operatorPolicies", ops), ("policies", policies)):
                if new[key] and value:
                    comp.insert(pos, key, value)
                    doc.place(comp, key, line)
            changed = True
        if changed:
            doc.save()


def merges(node):
    """Source lines of the maps under node that use a YAML merge key."""
    if isinstance(node, Map):
        own = [node.lc.line + 1] if getattr(node, merge_attrib, None) else []
        return own + [x for v in node.values() for x in merges(v)]
    if isinstance(node, Seq):
        return [x for v in node for x in merges(v)]
    return []


def files(paths):
    """Files under paths, skipping what git ignores."""
    out = set()
    for p in map(Path, paths):
        p = p.resolve()
        if p.is_file():
            out.add(p)
            continue
        r = git("ls-files", "-co", "--exclude-standard", cwd=p)
        out |= (
            {p / f for f in r.stdout.split()}
            if r.returncode == 0
            else set(p.rglob("*"))
        )
    return sorted(out)


def main(argv):
    root = Path(
        git("rev-parse", "--show-toplevel", cwd=Path.cwd()).stdout.strip() or "."
    )
    targets = files(argv or [root])
    every = files([root])
    charts = sorted(
        {
            f.parent
            for f in every
            if f.name == "Chart.yaml"
            and "name: policy-library" in f.read_text()
            and (f.parent / "values.yaml").exists()
        }
    )
    elements = [Element(c) for c in charts]
    models = {}
    for e in elements:
        models.update(e.models)
    chosen = {f.parent for f in targets if f.name == "Chart.yaml"} | {
        c for c in charts if any(c in f.parents for f in targets)
    }
    layers = [
        Layer(f, models)
        for f in targets
        if f.suffix in (".yaml", ".yml")
        and f.exists()
        and not any(c == f.parent or c in f.parents for c in charts)
        and re.search(r"^stack:", f.read_text(), re.MULTILINE)
    ]
    # ruamel cannot edit a map built with a YAML merge key; stop before writing anything
    todo = [
        (e.where, c) for e in elements if e.chart in chosen for c in e.stack.values()
    ]
    todo += [(la.path, c) for la in layers for c in la.stack.values()]
    if merged := [
        f"{path} line {line}: YAML merge key (<<); expand it by hand and rerun"
        for path, comp in todo
        if is_v3(comp)
        for line in merges(comp)
    ]:
        print("\n".join(merged))
        return 1
    for e in elements:
        if e.chart in chosen:
            e.migrate()
    for la in layers:
        la.migrate()
    for n in notes:
        print(n)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
