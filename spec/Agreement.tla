---------------------------- MODULE Agreement ----------------------------
(* 合意の判定のモデル。約束リストの項目の中身の変更、合意、合意の取り消し、ストーリーをまたいだ変更を操作とする。 *)
(* 中身の指紋は、版の番号で表す。版は進むだけで戻らない（同じ中身に戻すことは、このモデルでは扱わない）。 *)
EXTENDS Naturals

CONSTANTS Items, Stories, MaxVer

VARIABLES content, agreements, changedBy, agreedNow, last

vars == <<content, agreements, changedBy, agreedNow, last>>

\* @vanda:internal 最後に起きた操作を記録し、どの操作も一度は起きることを確かめるためだけの集合
Actions == {"none", "Change", "Agree", "Revoke"}

\* @vanda:term agreement-record
AgreementRecord == [item : Items, fp : 0..MaxVer, story : Stories]

TypeOK ==
    /\ content \in [Items -> 0..MaxVer]
    /\ agreements \subseteq AgreementRecord
    /\ changedBy \in [Items -> SUBSET Stories]
    /\ agreedNow \in [Items -> BOOLEAN]
    /\ last \in Actions

(* 今の中身の指紋に合う合意が 1 つでもあれば合意済 *)
Agreed(i) == \E a \in agreements : a.item = i /\ a.fp = content[i]

(* s の外れた合意: s が合意したことがあり、今の中身に s の合意がない項目。誰が変えたかを問わない（PO の判断） *)
\* @vanda:claim detached-agreement
Broken(s) ==
    {i \in Items :
        /\ \E a \in agreements : a.item = i /\ a.story = s
        /\ ~ \E a \in agreements : a.item = i /\ a.story = s /\ a.fp = content[i]}

(* s の範囲の合意の審査が通る: s が変えた項目がすべて合意済で、s の外れた合意がない *)
StoryPass(s) ==
    /\ \A i \in Items : s \in changedBy[i] => Agreed(i)
    /\ Broken(s) = {}

(* 全部の範囲の合意の審査が通る *)
AllPass == \A i \in Items : Agreed(i)

Init ==
    /\ content = [i \in Items |-> 0]
    /\ agreements = {}
    /\ changedBy = [i \in Items |-> {}]
    /\ agreedNow = [i \in Items |-> FALSE]
    /\ last = "none"

(* ストーリー s が項目 i の中身を変える *)
\* @vanda:claim old-agreements-kept-as-history
Change(s, i) ==
    /\ content[i] < MaxVer
    /\ content' = [content EXCEPT ![i] = @ + 1]
    /\ changedBy' = [changedBy EXCEPT ![i] = @ \cup {s}]
    /\ agreedNow' = [agreedNow EXCEPT ![i] = FALSE]
    /\ last' = "Change"
    /\ UNCHANGED agreements

(* ストーリー s で、項目 i の今の中身に合意する。合意の記録は消さない *)
Agree(s, i) ==
    /\ [item |-> i, fp |-> content[i], story |-> s] \notin agreements
    /\ agreements' = agreements \cup {[item |-> i, fp |-> content[i], story |-> s]}
    /\ agreedNow' = [agreedNow EXCEPT ![i] = TRUE]
    /\ last' = "Agree"
    /\ UNCHANGED <<content, changedBy>>

(* ストーリー s で、項目 i の今の中身への s の合意の記録を消す（RV-17）。フラグは戻さない（モデルにはフラグがない）。     *)
(* 取り消せる範囲（今の人が、今のブランチで書いた記録だけ）は、このモデルの粒度では表さない。                         *)
(* ほかのストーリーの、今の中身への合意が残っていれば、合意済のまま                                                    *)
\* @vanda:claim revoke-returns-to-proposed
Revoke(s, i) ==
    /\ [item |-> i, fp |-> content[i], story |-> s] \in agreements
    /\ agreements' = agreements \ {[item |-> i, fp |-> content[i], story |-> s]}
    /\ agreedNow' = [agreedNow EXCEPT ![i] = \E t \in Stories \ {s} : [item |-> i, fp |-> content[i], story |-> t] \in agreements]
    /\ last' = "Revoke"
    /\ UNCHANGED <<content, changedBy>>

Next == \E s \in Stories, i \in Items : Change(s, i) \/ Agree(s, i) \/ Revoke(s, i)

Spec == Init /\ [][Next]_vars

(* 性質 1: 操作ごとに付けた「合意済」と、合意の記録から判定した合意済が、いつも一致する *)
\* @vanda:claim agreed-when-fingerprint-matches
AgreedOnlyWithMatchingFingerprint == \A i \in Items : agreedNow[i] = Agreed(i)

(* 性質 2: s の外れた合意があるあいだ、s の範囲の審査は止まる *)
BrokenBlocksStory == \A s \in Stories : Broken(s) # {} => ~ StoryPass(s)

(* 性質 3: ほかのストーリーが今の中身に合意しても、s の外れた合意は外れたまま。s が合意し直せば解ける *)
(* 性質 4: s 自身が変えた項目も、s が前に合意していれば、s が合意し直すまで外れたまま *)
\* @vanda:claim detached-agreement
OwnChangeBreaks ==
    [][\A s \in Stories, i \in Items :
          (Change(s, i) /\ \E a \in agreements : a.item = i /\ a.story = s) => i \in Broken(s)']_vars

\* @vanda:claim detached-until-story-reagrees
OnlyOwnAgreementRepairs ==
    [][\A s, t \in Stories, i \in Items :
          /\ (Agree(t, i) /\ t # s /\ i \in Broken(s)) => i \in Broken(s)'
          /\ Agree(s, i) => i \notin Broken(s)']_vars

(* どの操作も一度は起きることを確かめるため、違反が見つかるはずの不変条件 *)
NeverChange == last # "Change"
NeverAgree == last # "Agree"
NeverRevoke == last # "Revoke"
NeverBroken == \A s \in Stories : Broken(s) = {}
=============================================================================
