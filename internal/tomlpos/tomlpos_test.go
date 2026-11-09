package tomlpos

import "testing"

func TestLines(t *testing.T) {
	src := `version = "0.1.0"  # バージョン

[[term]]
id = "task"
name = """
[[claim]]
"""
definition = 'x = [1'

[[claim]]
id = "a"
layers = [
  "model",
  "cli",
]
statement = "s"
[[claim.evidence]]
stage = "implementation"

[[claim.evidence]]
type = "unit-test"

[[claim]]
id = "b"
evidence = [{ stage = "implementation" }]

[story]
id_pattern = "#[0-9]+"
`
	want := map[string]int{
		"version":                    1,
		"term":                       3,
		"claim":                      10,
		"claim[0].evidence":          17,
		"term[0]":                    3,
		"term[0].id":                 4,
		"term[0].name":               5,
		"term[0].definition":         8,
		"claim[0]":                   10,
		"claim[0].id":                11,
		"claim[0].layers":            12,
		"claim[0].statement":         16,
		"claim[0].evidence[0]":       17,
		"claim[0].evidence[0].stage": 18,
		"claim[0].evidence[1]":       20,
		"claim[0].evidence[1].type":  21,
		"claim[1]":                   23,
		"claim[1].id":                24,
		"claim[1].evidence":          25,
		"story":                      27,
		"story.id_pattern":           28,
	}
	got := Lines([]byte(src))
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got line %d, want %d", k, got[k], v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("余分な場所 %s（%d 行目）を記録した", k, got[k])
		}
	}
}
