-------------------------- MODULE AgreementTrace --------------------------
(* クロスチェック（cmd/vanda/crosscheck_test.go）で、TLC -simulate にトレースを書き出させるための包み。    *)
(* Agreement のトレースには操作の引数も判定の値も出ないので、step に操作と引数を、obs にモデルの判定を、     *)
(* 1 歩ごとに書かせる。Agreement の操作と性質は変えない。                                                *)
EXTENDS Agreement

VARIABLES step, obs

Obs == [agreed    |-> [i \in Items |-> Agreed(i)],
        broken    |-> [s \in Stories |-> Broken(s)],
        storyPass |-> [s \in Stories |-> StoryPass(s)],
        allPass   |-> AllPass]

InitT == Init /\ step = <<"Init">> /\ obs = Obs

NextT ==
    /\ \E s \in Stories, i \in Items :
          \/ Change(s, i) /\ step' = <<"Change", s, i>>
          \/ Agree(s, i) /\ step' = <<"Agree", s, i>>
    /\ obs' = Obs'

SpecT == InitT /\ [][NextT]_<<vars, step, obs>>
=============================================================================
