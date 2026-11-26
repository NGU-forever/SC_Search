#!/usr/bin/env bash
# Report non-test Go file/function sizes under go/internal.
#
# Modes:
#   ./scripts/size_report.sh              # report only (always exits 0)
#   SIZE_STRICT=1 ./scripts/size_report.sh  # fail on any budget violation
#   SIZE_RATCHET=1 ./scripts/size_report.sh # fail on new/grown violations only
#   ./scripts/size_report.sh --ratchet      # same as SIZE_RATCHET=1
#
# The ratchet baseline lives in scripts/size_baseline.txt and lists only known,
# documented exceptions.
set -euo pipefail

if [ "${1:-}" = "--ratchet" ]; then
  export SIZE_RATCHET=1
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
top="${SIZE_TOP:-15}"

python3 - "$root" "$top" <<'PY'
import os
import re
import sys

root = sys.argv[1]
top_n = int(sys.argv[2])
internal = os.path.join(root, "go", "internal")
baseline_path = os.path.join(root, "scripts", "size_baseline.txt")
file_budget = 500
func_budget = 120


def mask_go(src: str) -> str:
    """Blank comments and string/rune literals while preserving byte length."""
    out = []
    i = 0
    n = len(src)
    state = "code"
    while i < n:
        c = src[i]
        if state == "code":
            if c == "/" and i + 1 < n and src[i + 1] == "/":
                out.append("  ")
                i += 2
                state = "line"
                continue
            if c == "/" and i + 1 < n and src[i + 1] == "*":
                out.append("  ")
                i += 2
                state = "block"
                continue
            if c == '"':
                out.append(" ")
                i += 1
                state = "string"
                continue
            if c == "'":
                out.append(" ")
                i += 1
                state = "rune"
                continue
            if c == "`":
                out.append(" ")
                i += 1
                state = "raw"
                continue
            out.append(c)
            i += 1
            continue
        if state == "line":
            out.append("\n" if c == "\n" else " ")
            if c == "\n":
                state = "code"
            i += 1
            continue
        if state == "block":
            if c == "*" and i + 1 < n and src[i + 1] == "/":
                out.append("  ")
                i += 2
                state = "code"
                continue
            out.append("\n" if c == "\n" else " ")
            i += 1
            continue
        if state in ("string", "rune"):
            if c == "\\":
                out.append("  ")
                i += 2
                continue
            if (state == "string" and c == '"') or (state == "rune" and c == "'"):
                out.append(" ")
                i += 1
                state = "code"
                continue
            out.append("\n" if c == "\n" else " ")
            i += 1
            continue
        if state == "raw":
            if c == "`":
                out.append(" ")
                i += 1
                state = "code"
                continue
            out.append("\n" if c == "\n" else " ")
            i += 1
            continue
    return "".join(out)


def body_brace(masked: str, start: int) -> int:
    """Return the index of the function body's opening brace.

    Tracks parenthesis/bracket depth and struct{}/interface{} type literals so a
    `map[string]struct{}` in the signature is not mistaken for the body. Returns
    -1 for a body-less declaration.
    """
    i = start
    n = len(masked)
    paren = 0
    bracket = 0
    lit = 0
    prev = ""
    while i < n:
        if masked.startswith("\nfunc ", i):
            return -1
        c = masked[i]
        if c.isalnum() or c == "_":
            j = i
            while j < n and (masked[j].isalnum() or masked[j] == "_"):
                j += 1
            prev = masked[i:j]
            i = j
            continue
        if c == "(":
            paren += 1
        elif c == ")":
            if paren > 0:
                paren -= 1
        elif c == "[":
            bracket += 1
        elif c == "]":
            if bracket > 0:
                bracket -= 1
        elif c == "{":
            if paren > 0 or bracket > 0 or prev in ("struct", "interface"):
                lit += 1
            else:
                return i
        elif c == "}":
            if lit > 0:
                lit -= 1
                prev = "}"
        elif not c.isspace():
            prev = c
        i += 1
    return -1


def func_name(header: str) -> str:
    header = header.strip()
    m = re.match(r"func\s*\(([^)]*)\)\s*([A-Za-z_][A-Za-z0-9_]*)", header)
    if m:
        recv = m.group(1).strip()
        recv = re.sub(r"\s*\*?\s*([A-Za-z_][A-Za-z0-9_]*)\s*$", r"\1", recv)
        if " " in recv:
            recv = recv.split()[-1]
        recv = recv.lstrip("*")
        return "(%s).%s" % (recv, m.group(2))
    m = re.match(r"func\s+([A-Za-z_][A-Za-z0-9_]*)", header)
    return m.group(1) if m else header


files = []
funcs = []
for dirpath, _dirnames, filenames in os.walk(internal):
    for name in filenames:
        if not name.endswith(".go") or name.endswith("_test.go"):
            continue
        path = os.path.join(dirpath, name)
        with open(path, "r", encoding="utf-8") as f:
            src = f.read()
        rel = os.path.relpath(path, root)
        files.append((len(src.splitlines()), rel))
        masked = mask_go(src)
        lines = masked.splitlines(keepends=True)
        offsets = [0]
        for line in lines:
            offsets.append(offsets[-1] + len(line))

        def line_of(pos: int) -> int:
            lo, hi = 0, len(offsets) - 1
            while lo < hi:
                mid = (lo + hi + 1) // 2
                if offsets[mid] <= pos:
                    lo = mid
                else:
                    hi = mid - 1
            return lo + 1

        for m in re.finditer(r"(?m)^func\s", masked):
            start = m.start()
            brace = body_brace(masked, start)
            if brace < 0:
                continue
            depth = 0
            end = -1
            for j in range(brace, len(masked)):
                ch = masked[j]
                if ch == "{":
                    depth += 1
                elif ch == "}":
                    depth -= 1
                    if depth == 0:
                        end = j
                        break
            if end < 0:
                continue
            header = src[start:brace]
            start_line = line_of(start)
            end_line = line_of(end)
            funcs.append((end_line - start_line + 1, rel, start_line, func_name(header)))

files.sort(reverse=True)
funcs.sort(reverse=True)

print("SC_Search size report (go/internal, non-test files)")
print("budgets: file <= %d lines, function <= %d lines" % (file_budget, func_budget))
print()
print("Top %d files by line count:" % top_n)
for count, rel in files[:top_n]:
    print("%7d  %s" % (count, rel))
print()
print("Top %d functions by line count:" % top_n)
for count, rel, line, name in funcs[:top_n]:
    print("%7d  %s  %s:%d" % (count, name, rel, line))
print()

file_violations = [(c, p) for c, p in files if c > file_budget]
func_violations = [(c, p, line, n) for c, p, line, n in funcs if c > func_budget]
print("Violations:")
if not file_violations:
    print("  files > %d lines: none" % file_budget)
else:
    print("  files > %d lines: %d" % (file_budget, len(file_violations)))
    for count, rel in sorted(file_violations, reverse=True):
        print("    %7d  %s" % (count, rel))
if not func_violations:
    print("  functions > %d lines: none" % func_budget)
else:
    print("  functions > %d lines: %d" % (func_budget, len(func_violations)))
    for count, rel, line, name in sorted(func_violations, reverse=True):
        print("    %7d  %s  %s:%d" % (count, name, rel, line))


def load_baseline(path):
    """Parse the ratchet baseline into (file, func) dicts."""
    base_files = {}
    base_funcs = {}
    if not os.path.exists(path):
        return None, None
    with open(path, "r", encoding="utf-8") as f:
        for raw in f:
            line = raw.split("#", 1)[0].strip()
            if not line:
                continue
            parts = line.split()
            if len(parts) == 3 and parts[0] == "file":
                base_files[parts[1]] = int(parts[2])
            elif len(parts) == 4 and parts[0] == "func":
                base_funcs[(parts[1], parts[2])] = int(parts[3])
            else:
                print("  baseline parse error: %s" % raw.rstrip())
                sys.exit(2)
    return base_files, base_funcs


ratchet = os.environ.get("SIZE_RATCHET") == "1"
strict = os.environ.get("SIZE_STRICT") == "1"
failed = False

if ratchet:
    base_files, base_funcs = load_baseline(baseline_path)
    print()
    print("Ratchet (baseline: scripts/size_baseline.txt):")
    if base_files is None:
        print("  FAIL: baseline file not found: %s" % baseline_path)
        failed = True
    else:
        problems = []
        for count, rel in sorted(file_violations, reverse=True):
            if rel not in base_files:
                problems.append("new file violation: %s (%d > %d)" % (rel, count, file_budget))
            elif count > base_files[rel]:
                problems.append("file grew: %s %d > baseline %d" % (rel, count, base_files[rel]))
        for count, rel, line, name in sorted(func_violations, reverse=True):
            key = (rel, name)
            if key not in base_funcs:
                problems.append("new function violation: %s (%s:%d, %d > %d)" % (name, rel, line, count, func_budget))
            elif count > base_funcs[key]:
                problems.append("function grew: %s (%s) %d > baseline %d" % (name, rel, count, base_funcs[key]))
        seen_files = {rel for _, rel in file_violations}
        seen_funcs = {(rel, name) for _, rel, _, name in func_violations}
        for rel, lines in sorted(base_files.items()):
            if rel not in seen_files:
                print("  note: baseline entry is now within budget and can be removed: %s (%d)" % (rel, lines))
        for (rel, name), lines in sorted(base_funcs.items()):
            if (rel, name) not in seen_funcs:
                print("  note: baseline entry is now within budget and can be removed: %s (%s, %d)" % (name, rel, lines))
        if problems:
            for p in problems:
                print("  FAIL: " + p)
            failed = True
        else:
            print("  OK: no new or grown violations (%d file / %d function exceptions)" % (len(base_files), len(base_funcs)))

if (strict and (file_violations or func_violations)) or failed:
    sys.exit(1)
PY
