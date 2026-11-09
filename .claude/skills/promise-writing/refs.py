#!/usr/bin/env python3
"""約束リストの、文字のまま書いた用語・機能の名前を、参照（{@id:<ID>}）に書き換えるための道具。

  python3 refs.py candidates <約束リスト> [<外す候補の JSONL>]
                                                     候補を 1 行に 1 つの JSON で出す。外す候補を渡すと、その字を空けて当て直す
  python3 refs.py apply <約束リスト> <採る候補の JSONL>  採る候補を書き換える（ファイルを上書きする）
  python3 refs.py verify <前の約束リスト> <後の約束リスト> 参照を見せる文字に置き換えた文が、前と同じかを確かめる

名前は長い順に当て、当たった所は短い名前に当て直さない（「合意の記録」の中の「合意」を候補にしない）。
長い名前の候補を判定で外したら、その候補を渡して当て直す。端が重なる名前（「審査の設定のファイル」の「設定のファイル」と「審査の設定」）は、
長いほうに隠れて候補に出ないため。
当たったものが本当にその用語・機能を指しているかは、このスクリプトでは決めない（haiku と親が 1 組ずつ決める）。
約束リストの文字列はすべて 1 行の基本文字列（"…"）で書いてある前提で、行の中の位置で書き換える。
"""
import json
import re
import sys
import tomllib

# 名前を除く、自然言語で書く項目（約束 reference-form）。evidence の detail・formal_waiver も含める
NL_KEYS = {"definition", "formal_waiver", "statement", "rationale", "condition", "guesses", "model_check_waiver", "detail"}
LINE_RE = re.compile(r'^([a-z_]+) = (.*)$')
STR_RE = re.compile(r'"((?:[^"\\]|\\.)*)"')


def load_names(path):
    with open(path, "rb") as f:
        doc = tomllib.load(f)
    names = {}
    for kind in ("term", "feature"):
        for item in doc.get(kind, []):
            names[item["id"]] = (kind, item["name"])
    claims = {c["id"] for c in doc.get("claim", [])}
    return names, claims


def unescape(s):
    return json.loads('"' + s + '"')


def nl_strings(path):
    """(行の番号, 項目, 行の中の文字列の番号, 生の文字列, その行が属する項目の ID) を返す。"""
    owner = None
    with open(path, encoding="utf-8") as f:
        for no, line in enumerate(f, 1):
            line = line.rstrip("\n")
            m = LINE_RE.match(line)
            if not m:
                continue
            key, rest = m.groups()
            if key == "id":
                owner = unescape(STR_RE.match(rest).group(1))
                continue
            if key not in NL_KEYS:
                continue
            for i, sm in enumerate(STR_RE.finditer(rest)):
                yield no, key, i, sm.group(1), owner


def ref_spans(raw):
    """生の文字列の中の、参照と {@@ の範囲（書き換えない所）。"""
    spans = []
    i = 0
    while True:
        j = raw.find("{@", i)
        if j < 0:
            return spans
        if raw.startswith("{@@", j):
            spans.append((j, j + 3))
            i = j + 3
            continue
        k = raw.find("}", j)
        k = len(raw) if k < 0 else k + 1
        spans.append((j, k))
        i = k


def candidates(path, skip=()):
    names, _ = load_names(path)
    by_len = sorted(names.items(), key=lambda kv: -len(kv[1][1]))
    out = []
    for no, key, si, raw, owner in nl_strings(path):
        taken = ref_spans(raw)
        found = []
        for tid, (kind, name) in by_len:
            start = 0
            while True:
                p = raw.find(name, start)
                if p < 0:
                    break
                q = p + len(name)
                if (no, si, p, name) in skip:
                    pass
                elif all(q <= a or p >= b for a, b in taken):
                    taken.append((p, q))
                    if tid != owner:
                        found.append((p, q, tid, kind, name))
                start = p + 1
        for p, q, tid, kind, name in sorted(found):
            out.append({
                "line": no, "key": key, "string": si, "start": p, "end": q,
                "owner": owner, "target": tid, "kind": kind, "name": name,
                "text": unescape(raw),
                "marked": unescape(raw[:p]) + "【" + name + "】" + unescape(raw[q:]),
            })
    return out


def apply(path, picks):
    by_string = {}
    for c in picks:
        by_string.setdefault((c["line"], c["string"]), []).append(c)
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")
    for (no, si), cs in by_string.items():
        line = lines[no - 1]
        m = LINE_RE.match(line)
        prefix = len(line) - len(m.group(2))
        sm = list(STR_RE.finditer(m.group(2)))[si]
        raw = sm.group(1)
        for c in sorted(cs, key=lambda c: -c["start"]):
            if raw[c["start"]:c["end"]] != c["name"]:
                sys.exit(f"{path}:{no}: 候補の位置に {c['name']} がありません")
            raw = raw[:c["start"]] + "{@id:" + c["target"] + "}" + raw[c["end"]:]
        a, b = prefix + sm.start(1), prefix + sm.end(1)
        lines[no - 1] = line[:a] + raw + line[b:]
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))


def shown(raw, names):
    """参照を見せる文字（用語・機能は名前、約束は ID）に、{@@ を {@ に置き換えた文。"""
    out, i = [], 0
    while True:
        j = raw.find("{@", i)
        if j < 0:
            out.append(raw[i:])
            return "".join(out)
        out.append(raw[i:j])
        if raw.startswith("{@@", j):
            out.append("{@")
            i = j + 3
            continue
        k = raw.find("}", j)
        kind, _, val = raw[j + 2:k].partition(":")
        if k < 0 or kind != "id" or not val:
            sys.exit(f"壊れた参照: {raw[j:]}")
        out.append(names[val][1] if val in names else val)
        i = k + 1


def verify(before, after):
    nb, _ = load_names(before)
    na, claims = load_names(after)
    if nb != na:
        sys.exit("用語・機能の ID か名前が変わっています")
    a = [(no, k, si, shown(unescape(r), na)) for no, k, si, r, _ in nl_strings(after)]
    b = [(no, k, si, shown(unescape(r), nb)) for no, k, si, r, _ in nl_strings(before)]
    if len(a) != len(b):
        sys.exit(f"自然言語の項目の数が違います（前 {len(b)}、後 {len(a)}）")
    bad = 0
    for x, y in zip(b, a):
        if x != y:
            bad += 1
            print(f"{after}:{y[0]}: 前と違います\n  前: {x[3]}\n  後: {y[3]}")
    with open(before, encoding="utf-8") as f:
        lb = f.read().split("\n")
    with open(after, encoding="utf-8") as f:
        la = f.read().split("\n")
    nl_lines = {no for no, *_ in a}
    for i, (x, y) in enumerate(zip(lb, la), 1):
        if x != y and i not in nl_lines:
            bad += 1
            print(f"{after}:{i}: 自然言語の項目でない行が変わっています")
    if len(lb) != len(la):
        bad += 1
        print("行の数が変わっています")
    refs = sum(len(re.findall(r"\{@id:", r)) for *_, r, _ in nl_strings(after)) - sum(len(re.findall(r"\{@id:", r)) for *_, r, _ in nl_strings(before))
    print(f"自然言語の文字列 {len(a)} 個を比べました。違うもの {bad} 個。足した参照 {refs} 個")
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "candidates" and len(sys.argv) in (3, 4):
        skip = set()
        if len(sys.argv) == 4:
            with open(sys.argv[3], encoding="utf-8") as f:
                skip = {(c["line"], c["string"], c["start"], c["name"]) for c in map(json.loads, f) if c}
        for c in candidates(sys.argv[2], skip):
            print(json.dumps(c, ensure_ascii=False))
    elif cmd == "apply" and len(sys.argv) == 4:
        with open(sys.argv[3], encoding="utf-8") as f:
            apply(sys.argv[2], [json.loads(l) for l in f if l.strip()])
    elif cmd == "verify" and len(sys.argv) == 4:
        verify(sys.argv[2], sys.argv[3])
    else:
        sys.exit(__doc__)
