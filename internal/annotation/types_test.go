package annotation

import (
	"fmt"
	"testing"
)

// @vanda:evidence annotation-links-tla-types
func TestTLATypesはTypeOKの中でinとsubseteqの右にある定義を型とみなす(t *testing.T) {
	src := `---- MODULE A ----
EXTENDS Naturals
CONSTANT Ids
Status == {"todo", "done"}
Task == [id : Ids, status : Status]
Pairs == Task \X Task
Helper(x) == x
Unused == {}
VARIABLES tasks, links, seen
TypeOK ==
    /\ tasks \subseteq Task /\ seen \in BOOLEAN
    /\ links \in [Ids -> SUBSET Pairs] \/ Helper(1) = 1
Init == tasks = {}
====
`
	got := fmt.Sprint(TLATypes([]byte(src)))
	want := "[{Task 5} {Pairs 6}]"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestTLATypesはTypeOKがなければ何も返さない(t *testing.T) {
	if got := TLATypes([]byte("---- MODULE A ----\nS == {1}\nInv == 1 \\in S\n====\n")); got != nil {
		t.Errorf("got %v", got)
	}
}
