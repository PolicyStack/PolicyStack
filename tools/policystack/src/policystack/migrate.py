"""Upgrade steps: declarative rename and move ops on adopter YAML under values/ and fleet/.

Ops edit a ruamel round-trip tree. Only lines the edit changed are carried back onto the original
text (3-way: original, dump of the untouched tree, dump of the edited tree), and inside a changed
line only the changed characters, so formatting ruamel normalises (null spelling, quoting, CRLF,
flow-map spacing) survives everywhere outside the edit. The result is re-parsed and must equal the
edited tree, otherwise the file is refused for a manual edit.
"""

import difflib
import io
import os
import re
from pathlib import Path

from ruamel.yaml import YAML, YAMLError
from ruamel.yaml.comments import CommentedMap

ADOPTER = ("values", "fleet")


def _yaml():
    y = YAML()
    y.preserve_quotes = True
    y.width = 4096
    y.indent(mapping=2, sequence=4, offset=2)
    return y


def _split(path):
    return path if isinstance(path, list) else path.split(".")


def stack_keys(ops):
    """The stack.<key> each op rewrites ('*' for any): the elements whose values the step changes."""
    paths = (_split(op.get("rename") or op["move"]) for op in ops)
    return {p[1] for p in paths if p[0] == "stack" and len(p) > 1}


def _match(node, path, keys=()):
    """Yield (chain of mappings from the root, matched keys) for each own key matching path ('*' = any).

    Keys inherited through a '<<' merge are not matched: editing them would edit the anchor."""
    head, rest = path[0], path[1:]
    if not isinstance(node, dict):
        return
    own = [k for k, _ in node.non_merged_items()]
    for k in own if head == "*" else [head] if head in own else []:
        if rest:
            for chain, ks in _match(node[k], rest, keys + (k,)):
                yield [node, *chain], ks
        else:
            yield [node], keys + (k,)


def _fill(path, src, pattern):
    caps = [k for k, p in zip(src, pattern) if p == "*"]
    return [re.sub(r"\$(\d)", lambda m: str(caps[int(m[1]) - 1]), p) for p in path]


def _apply(doc, op):
    kind = "rename" if "rename" in op else "move"
    pattern = _split(op[kind])
    for chain, keys in list(_match(doc, pattern)):
        parent, k = chain[-1], keys[-1]
        dest = _fill(_split(op["to"]), keys, pattern)
        if kind == "rename":
            dest = [*keys[:-1], dest[0]]
        pos, comment = list(parent).index(k), parent.ca.items.pop(k, None)
        value = parent.pop(k)
        node = doc
        for depth, d in enumerate(dest[:-1]):
            if d not in node:  # a new key takes the place of the branch it replaces
                at = pos if node is parent else len(node)
                if node is not parent and depth < len(chain) and node is chain[depth]:
                    at = list(node).index(keys[depth])
                node.insert(at, d, CommentedMap())
            node = node[d]
        if dest[-1] in node:
            raise ValueError(f"{'.'.join(keys)} -> {'.'.join(dest)}: target already set")
        node.insert(pos if node is parent else len(node), dest[-1], value)
        if comment:
            node.ca.items[dest[-1]] = comment


def _dump(y, docs, explicit):
    y.explicit_start = explicit
    buf = io.StringIO()
    y.dump_all(docs, buf)
    return buf.getvalue().splitlines(True)


def _carry(orig, before, after):
    """Return orig with the before->after line edits applied; inside equal-sized hunks, per character."""
    out = orig[:]
    ops = difflib.SequenceMatcher(None, before, after, autojunk=False).get_opcodes()
    for tag, i1, i2, j1, j2 in reversed(ops):
        if tag == "equal":
            continue
        if tag == "replace" and i2 - i1 == j2 - j1:
            out[i1:i2] = map(_line, orig[i1:i2], before[i1:i2], after[j1:j2])
        else:
            out[i1:i2] = after[j1:j2]
    return out


def _line(o, b, a):
    """Apply the b->a character edits to o when they all sit in the prefix o and b share."""
    if o == b:
        return a
    p = len(os.path.commonprefix([o, b]))
    ops = difflib.SequenceMatcher(None, b, a, autojunk=False).get_opcodes()
    edits = [x for x in ops if x[0] != "equal"]
    if edits and edits[-1][2] <= p:  # every edit sits in the part orig and before share
        for _, i1, i2, j1, j2 in reversed(edits):
            o = o[:i1] + a[j1:j2] + o[i2:]
        return o
    return a


def migrate(src, ops):
    """Return src with ops applied, or src itself (same object) when no op matched."""
    y = _yaml()
    explicit = re.match(r"(?:[ \t]*(?:#.*)?\n)*---", src) is not None
    before = _dump(y, list(y.load_all(src)), explicit)
    docs = list(y.load_all(src))
    for doc in docs:
        for op in ops:
            _apply(doc, op)
    after = _dump(y, docs, explicit)
    if after == before:
        return src
    orig = src.splitlines(True)
    if len(orig) != len(before):
        raise ValueError("file does not round-trip line for line; edit it by hand")
    out = "".join(_carry(orig, before, after))
    safe = YAML(typ="safe")
    if list(safe.load_all(out)) != list(safe.load_all("".join(after))):
        raise ValueError("patched text does not match the edit; edit it by hand")
    return out


def targets(root, globs, tracked):
    """Regular files under values/ or fleet/ that globs match and tracked (repository-relative paths,
    from git ls-files) holds. root is resolved. A pattern or match that leaves values/ and fleet/,
    or goes through a symlink, refuses the step instead of being skipped."""
    out = set()
    for g in globs:
        if Path(g).is_absolute() or ".." in Path(g).parts:
            raise ValueError(f"{g}: pattern leaves the repository")
        for p in root.glob(g):
            rel = p.relative_to(root)
            if rel.parts[0] not in ADOPTER or p.is_symlink() or p.resolve() != p:
                raise ValueError(f"{rel}: outside values/ and fleet/, or a symlink")
            if p.is_file() and rel.as_posix() in tracked:
                out.add(p)
    return sorted(out)


def apply(root, steps, tracked):
    """Run each (name, step) in order over its files, all in memory. Returns {path: new text} for the
    files that changed; raises ValueError naming the step and file before anything is written."""
    texts, root = {}, Path(root).resolve()
    for name, step in steps:
        for p in targets(root, step.get("files") or [], tracked) if step.get("ops") else ():
            src = texts.get(p) or p.read_bytes().decode()
            try:
                out = migrate(src, step["ops"])
            except (ValueError, YAMLError) as e:
                where = f"{name}: {p.relative_to(root)}"
                raise ValueError(f"{where}: {e}\n{step.get('manual', '')}") from e
            if out is not src:
                texts[p] = out
    return texts
