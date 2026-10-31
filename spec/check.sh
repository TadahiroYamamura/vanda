#!/bin/sh
# TLA+ のモデルを TLC で検査する。
# 性質の検査に加えて、どの操作も一度は起きることを確かめる（Never* の不変条件が、どれも破られるはず）。
# 矛盾した前提で、性質が空振りで成り立つのを見つけるため。
set -eu
cd "$(dirname "$0")"
meta=$(mktemp -d)
trap 'rm -rf "$meta"' EXIT

run() {
	tlc -noGenerateSpecTE -workers auto -metadir "$meta/$1" -config "$1" Agreement.tla >"$meta/out" 2>&1
}

if ! run Agreement.cfg; then
	cat "$meta/out"
	echo "Agreement.cfg: TLC の検査が通らなかった" >&2
	exit 1
fi
grep -E 'states generated|No error' "$meta/out"

for x in NeverChange NeverAgree NeverBroken; do
	if run "Agreement.$x.cfg"; then
		echo "Agreement.$x.cfg: $x が破られなかった（その操作や状態が一度も起きていない）" >&2
		exit 1
	fi
	if ! grep -q "Invariant $x is violated" "$meta/out"; then
		cat "$meta/out"
		echo "Agreement.$x.cfg: $x のほかの理由で TLC が失敗した" >&2
		exit 1
	fi
	echo "$x: 起きることを確かめた"
done
