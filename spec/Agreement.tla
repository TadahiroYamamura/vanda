---------------------------- MODULE Agreement ----------------------------
(* 合意の判定のモデル。約束リストの項目と合意の対象のファイル（まとめて「項目」）の中身の変更・追加・削除・名前の変更、 *)
(* 合意（消したことへの合意を含む）、合意の取り消し、ストーリーをまたいだ変更を操作とする。                              *)
(* 項目は、鍵（約束・用語・機能は種類と ID、ファイルは場所）で見分ける。Items の 1 つが 1 つの鍵。                       *)
(* 中身の指紋は 1..MaxFp の数で表す。数は項目をまたいで共通で、別の項目が同じ数なら同じ中身（写し）。                    *)
(* 項目ごとの版の番号にしないのは、写しや名前の変更で、別の場所に同じ中身ができる場面を起こすため。                      *)
(* 同じ理由で、中身を前の中身に戻すことも起きる。                                                                        *)
(* 範囲 story の比べる元（分岐点）は、はじめの中身とする。どのストーリーも、はじめの状態から分かれたものとして扱う。  *)
EXTENDS Naturals

CONSTANTS Items, Files, Stories, MaxFp

ASSUME Files \subseteq Items

VARIABLES content, base, agreements, changedBy, agreedNow, last

vars == <<content, base, agreements, changedBy, agreedNow, last>>

(* changedBy（項目を変えたストーリー）は判定に使わない。誰が変えたかを問わない場面が起きることを数えるためだけに持つ *)

(* 項目が今ないことを表す中身。消したことへの合意の記録は、指紋の代わりにこの値を持つ（実装では deleted = true） *)
Absent == 0
Fps == 1..MaxFp

\* @vanda:internal 最後に起きた操作を記録し、どの操作も一度は起きることを確かめるためだけの集合
Actions == {"none", "Change", "Add", "Delete", "Rename", "Agree", "Revoke"}

\* @vanda:term agreement-record
AgreementRecord == [item : Items, fp : 0..MaxFp, story : Stories]

TypeOK ==
    /\ content \in [Items -> 0..MaxFp]
    /\ base \in [Items -> 0..MaxFp]
    /\ agreements \subseteq AgreementRecord
    /\ changedBy \in [Items -> SUBSET Stories]
    /\ agreedNow \in [Items -> BOOLEAN]
    /\ last \in Actions

Present(i) == content[i] # Absent

(* 項目 i の中身 f への合意の記録がある。f = Absent なら、i を消したことへの合意の記録 *)
HasRecord(i, f) == \E a \in agreements : a.item = i /\ a.fp = f

(* 今の中身の指紋に合う、同じ項目への合意が 1 つでもあれば合意済。今ない項目は、消したことへの合意があれば合意済。        *)
(* 別の項目への合意は、指紋が同じでも数えない（ファイルは場所でも見分ける）                                              *)
Agreed(i) == HasRecord(i, content[i])

(* 中身に合意したことがある（消したことへの合意は数えない） *)
EverAgreed(i) == \E a \in agreements : a.item = i /\ a.fp # Absent

(* 項目 i が合意の審査で止まらない。今ある項目は合意済であること。今ない項目は、合意したことがあれば、             *)
(* 消したことへの合意があること。一度も合意していないまま消したものは、合意の対象がなかったので止めない             *)
\* @vanda:claim deleted-item-needs-agreement
Passes(i) == IF Present(i) THEN Agreed(i) ELSE (EverAgreed(i) => HasRecord(i, Absent))

(* s の外れた合意: s が中身に合意したことがあり、今の中身に s の合意がない項目。誰が変えたかを問わない（PO の判断）。      *)
(* 今ない項目（名前を変えた後の古い場所を含む）は、外れた合意にしない。消したことは、外れた合意でなく Passes で止める。   *)
(* レビューで、今ないものを外れた合意として出さない（review-detached-shown）ことに合わせる（PO の決定）。                *)
(* s が消したことにだけ合意した項目を足し直しても、s の外れた合意にしない（PO の決定）                                   *)
\* @vanda:claim detached-agreement
Broken(s) ==
    {i \in Items :
        /\ Present(i)
        /\ \E a \in agreements : a.item = i /\ a.story = s /\ a.fp # Absent
        /\ ~ \E a \in agreements : a.item = i /\ a.story = s /\ a.fp = content[i]}

(* 比べる元と今で違う項目（中身が違うもの、比べる元にあって今ないもの、今あって比べる元にないもの）。             *)
(* 変えて元に戻したものや、ストーリーの中で足して消したものは入らない                                            *)
Diff == {i \in Items : content[i] # base[i]}

(* s の範囲の合意の審査が通る: 比べる元と今で違う項目が、どれも今の状態（中身か、ないこと）で合意済で、s の外れた合意がない。 *)
(* 全部の範囲（Passes）と違い、比べる元にあって今ない項目は、合意したことがなくても、消したことへの合意を求める。          *)
(* 合意したことのあるものだけに求めると、PO が一度も見ていない削除で通り、story-pass-only-when-seen に反する（TLC の反例） *)
\* @vanda:claim check-agreement-story-scope
StoryPass(s) ==
    /\ \A i \in Diff : Agreed(i)
    /\ Broken(s) = {}

(* 全部の範囲の合意の審査が通る *)
AllPass == \A i \in Items : Passes(i)

(* はじめは、項目ごとに、ないか、どれかの中身がある。合意の記録はない。比べる元は、はじめの中身のまま変わらない *)
Init ==
    /\ content \in [Items -> 0..MaxFp]
    /\ base = content
    /\ agreements = {}
    /\ changedBy = [i \in Items |-> {}]
    /\ agreedNow = [i \in Items |-> FALSE]
    /\ last = "none"

(* ストーリー s が、今ある項目 i の中身を f に変える。f は、ほかの項目の中身や、i の前の中身と同じでもよい *)
Change(s, i, f) ==
    /\ Present(i)
    /\ f # content[i]
    /\ content' = [content EXCEPT ![i] = f]
    /\ changedBy' = [changedBy EXCEPT ![i] = @ \cup {s}]
    /\ agreedNow' = [agreedNow EXCEPT ![i] = HasRecord(i, f)]
    /\ last' = "Change"
    /\ UNCHANGED <<base, agreements>>

(* ストーリー s が、今ない項目 i を、中身 f で足す。前に消した項目を足し直すことも含む *)
Add(s, i, f) ==
    /\ ~ Present(i)
    /\ content' = [content EXCEPT ![i] = f]
    /\ changedBy' = [changedBy EXCEPT ![i] = @ \cup {s}]
    /\ agreedNow' = [agreedNow EXCEPT ![i] = HasRecord(i, f)]
    /\ last' = "Add"
    /\ UNCHANGED <<base, agreements>>

(* ストーリー s が、今ある項目 i を消す。i への合意の記録は消さない *)
Delete(s, i) ==
    /\ Present(i)
    /\ content' = [content EXCEPT ![i] = Absent]
    /\ changedBy' = [changedBy EXCEPT ![i] = @ \cup {s}]
    /\ agreedNow' = [agreedNow EXCEPT ![i] = HasRecord(i, Absent)]
    /\ last' = "Delete"
    /\ UNCHANGED <<base, agreements>>

(* ストーリー s が、ファイル i の名前を j に変える。i を消し、j に同じ中身を足すのを、1 つのコミットで行う。         *)
(* j の合意済は j への合意の記録だけで決まり、i への合意は j に移らない                                                *)
Rename(s, i, j) ==
    /\ i \in Files /\ j \in Files
    /\ Present(i)
    /\ ~ Present(j)
    /\ content' = [content EXCEPT ![i] = Absent, ![j] = content[i]]
    /\ changedBy' = [changedBy EXCEPT ![i] = @ \cup {s}, ![j] = @ \cup {s}]
    /\ agreedNow' = [agreedNow EXCEPT ![i] = HasRecord(i, Absent), ![j] = HasRecord(j, content[i])]
    /\ last' = "Rename"
    /\ UNCHANGED <<base, agreements>>

(* ストーリー s で、項目 i の今の中身に合意する。i が今なければ、消したことに合意する。合意の記録は消さない *)
Agree(s, i) ==
    /\ [item |-> i, fp |-> content[i], story |-> s] \notin agreements
    /\ agreements' = agreements \cup {[item |-> i, fp |-> content[i], story |-> s]}
    /\ agreedNow' = [agreedNow EXCEPT ![i] = TRUE]
    /\ last' = "Agree"
    /\ UNCHANGED <<content, base, changedBy>>

(* ストーリー s で、項目 i の今の中身への s の合意の記録を消す（RV-17）。i が今なければ、消したことへの合意を消す。       *)
(* フラグは戻さない（モデルにはフラグがない）。                                                                          *)
(* 取り消せる範囲（今の人が、今のブランチで書いた記録だけ）は、このモデルの粒度では表さない。                         *)
(* ほかのストーリーの、今の中身への合意が残っていれば、合意済のまま                                                    *)
\* @vanda:claim revoke-returns-to-proposed
Revoke(s, i) ==
    /\ [item |-> i, fp |-> content[i], story |-> s] \in agreements
    /\ agreements' = agreements \ {[item |-> i, fp |-> content[i], story |-> s]}
    /\ agreedNow' = [agreedNow EXCEPT ![i] = \E t \in Stories \ {s} : [item |-> i, fp |-> content[i], story |-> t] \in agreements]
    /\ last' = "Revoke"
    /\ UNCHANGED <<content, base, changedBy>>

(* 操作で項目の中身（今ないことを含む）が変わる *)
Edit(s) ==
    \/ \E i \in Items, f \in Fps : Change(s, i, f) \/ Add(s, i, f)
    \/ \E i \in Items : Delete(s, i)
    \/ \E i, j \in Items : Rename(s, i, j)

Next == \E s \in Stories : Edit(s) \/ \E i \in Items : Agree(s, i) \/ Revoke(s, i)

Spec == Init /\ [][Next]_vars

(* 性質 1: 操作ごとに付けた「合意済」と、合意の記録から判定した合意済が、いつも一致する *)
\* @vanda:claim agreed-when-fingerprint-matches
AgreedOnlyWithMatchingFingerprint == \A i \in Items : agreedNow[i] = Agreed(i)

(* 性質 2: s の外れた合意があるあいだ、s の範囲の審査は止まる *)
BrokenBlocksStory == \A s \in Stories : Broken(s) # {} => ~ StoryPass(s)

(* 性質 3: ほかのストーリーが今の中身に合意しても、s の外れた合意は外れたまま。s が合意し直せば解ける *)
(* 性質 4: s 自身が変えた項目も、s が前に中身に合意していれば、s が合意し直すまで外れたまま *)
\* @vanda:claim detached-agreement
OwnChangeBreaks ==
    [][\A s \in Stories, i \in Items, f \in Fps :
          (Change(s, i, f) /\ \E a \in agreements : a.item = i /\ a.story = s /\ a.fp # Absent) =>
              (i \in Broken(s)' <=> ~ [item |-> i, fp |-> f, story |-> s] \in agreements)]_vars

\* @vanda:claim detached-until-story-reagrees
OnlyOwnAgreementRepairs ==
    [][\A s, t \in Stories, i \in Items :
          /\ (Agree(t, i) /\ t # s /\ i \in Broken(s)) => i \in Broken(s)'
          /\ Agree(s, i) => i \notin Broken(s)']_vars

(* 性質 5: 取り消しのほかの操作（中身の変更・追加・削除・名前の変更・合意）は、合意の記録を消さない。         *)
(* 指紋の合わなくなった記録も、今ない項目への記録も、履歴として残る                                            *)
\* @vanda:claim old-agreements-kept-as-history
OnlyRevokeRemovesAgreements ==
    [][\A s \in Stories : (Edit(s) \/ \E i \in Items : Agree(s, i)) => agreements \subseteq agreements']_vars

(* 性質 6: 合意したことのある項目が今なく、消したことへの合意がなければ、全部の範囲の審査は止まる。         *)
(* 比べる元にあった項目なら、どのストーリーの範囲の審査も止まる。消したことに誰かが合意すれば、その項目では止まらない *)
\* @vanda:claim deleted-item-needs-agreement
DeletedBlocksWithoutAgreement ==
    \A i \in Items :
        /\ (~ Present(i) /\ EverAgreed(i) /\ ~ HasRecord(i, Absent)) =>
               /\ ~ AllPass
               /\ base[i] # Absent => \A s \in Stories : ~ StoryPass(s)
        /\ (~ Present(i) /\ HasRecord(i, Absent)) => Passes(i)

(* 性質 7: 名前を変えると、古い場所は消したものになり、新しい場所は、新しい場所への合意の記録がなければ、   *)
(* 中身が同じでも提案中になる（古い場所への合意は移らない）                                                *)
\* @vanda:claim renamed-file-needs-agreement
RenameNeedsAgreement ==
    [][\A s \in Stories, i, j \in Items :
          Rename(s, i, j) =>
              /\ ~ Present(i)'
              /\ (Passes(i)' <=> (EverAgreed(i) => HasRecord(i, Absent)))
              /\ (~ HasRecord(j, content[i]) => ~ Agreed(j)')]_vars

(* 性質 8: 別の項目が同じ中身で合意済でも（写し）、合意済になるのは、その項目への合意の記録があるときだけ *)
\* @vanda:claim agreed-when-fingerprint-matches
CopyIsNotAgreed ==
    \A i, j \in Items :
        (i # j /\ Present(j) /\ content[i] = content[j] /\ Agreed(i) /\ Agreed(j)) => HasRecord(j, content[j])

(* 性質 9: 全部の範囲の審査が通るなら、今ある項目の今の中身と、合意したことのある項目が今ないことは、どれも、   *)
(* 合意の記録（取り消したものを除く）のどれかが合意したもの                                                    *)
\* @vanda:claim pass-only-when-all-seen
PassOnlyWhenAllSeen ==
    AllPass =>
        \A i \in Items :
            /\ Present(i) => HasRecord(i, content[i])
            /\ (~ Present(i) /\ EverAgreed(i)) => HasRecord(i, Absent)

(* 性質 10: s の範囲の審査が通るなら、比べる元と今で違う項目は、どれも今の状態（今の中身か、ないこと）に、 *)
(* 合意の記録（取り消したものを除く）のどれかが合意したもの                                               *)
\* @vanda:claim story-pass-only-when-seen
StoryPassOnlyWhenSeen ==
    \A s \in Stories : StoryPass(s) => \A i \in Items : content[i] # base[i] => HasRecord(i, content[i])

(* どの操作も一度は起きることを確かめるため、違反が見つかるはずの不変条件 *)
NeverChange == last # "Change"
NeverAdd == last # "Add"
NeverDelete == last # "Delete"
NeverRename == last # "Rename"
NeverAgree == last # "Agree"
NeverAgreeDeletion == ~ \E a \in agreements : a.fp = Absent
NeverRevoke == last # "Revoke"
NeverBroken == \A s \in Stories : Broken(s) = {}

(* Agreement.cfg の検査で、last と changedBy だけが違う状態を同じとみなすための見方。この 2 つは、次の操作にも、  *)
(* Agreement.cfg の性質にも効かない（場面を数えるためだけにある）ので、同じとみなしても検査の結果は変わらず、     *)
(* 状態の数が減る。Never* と AgreementVacuity の検査は、この 2 つを見るので使わない                          *)
StateView == <<content, base, agreements, agreedNow>>
=============================================================================
