-------------------------- MODULE AgreementTrace --------------------------
(* クロスチェック（cmd/vanda/crosscheck_test.go）で、TLC -simulate にトレースを書き出させるための包み。    *)
(* Agreement のトレースには操作の引数も判定の値も出ないので、step に操作と引数を、obs にモデルの判定を、     *)
(* 1 歩ごとに書かせる。Agreement の操作と性質は変えない。                                                *)
(* step は <<操作, ストーリー, 項目>> で、Change・Add は 4 つ目に中身のフィンガープリントを、Rename は 4 つ目に新しい場所を持つ。 *)
(* 今ない項目では、承認済（agreed）とチェックで止まらないこと（passes）が別になるので、両方を出す                  *)
EXTENDS Agreement

VARIABLES step, obs

Obs == [agreed    |-> [i \in Items |-> Agreed(i)],
        passes    |-> [i \in Items |-> Passes(i)],
        broken    |-> [s \in Stories |-> Broken(s)],
        storyPass |-> [s \in Stories |-> StoryPass(s)],
        allPass   |-> AllPass]

InitT == Init /\ step = <<"Init">> /\ obs = Obs

NextT ==
    /\ \E s \in Stories :
          \/ \E i \in Items, f \in Fps :
                \/ Change(s, i, f) /\ step' = <<"Change", s, i, f>>
                \/ Add(s, i, f) /\ step' = <<"Add", s, i, f>>
          \/ \E i \in Items :
                \/ Delete(s, i) /\ step' = <<"Delete", s, i>>
                \/ Agree(s, i) /\ step' = <<"Agree", s, i>>
                \/ Revoke(s, i) /\ step' = <<"Revoke", s, i>>
          \/ \E i, j \in Items : Rename(s, i, j) /\ step' = <<"Rename", s, i, j>>
    /\ obs' = Obs'

SpecT == InitT /\ [][NextT]_<<vars, step, obs>>
=============================================================================
