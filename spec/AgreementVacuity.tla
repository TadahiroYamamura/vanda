------------------------- MODULE AgreementVacuity -------------------------
(* Agreement の性質が空振りで成り立っていないかを確かめるための、破られるはずの性質。               *)
(* どれも、TLC が反例を見つける（その状態や操作が一度は起きる）ことを、spec/agreement_vacuity_test.go で確かめる。 *)
(* Agreement.tla の Never* が確かめていない、性質の前提と RV-14 の場面を足す。                              *)
EXTENDS Agreement

(* StoryPass が、自分の変えた項目があっても成り立つことがある（BrokenBlocksStory の後件が空振りしない） *)
NeverOwnChangePasses ==
    \A s \in Stories : (\E i \in Items : s \in changedBy[i]) => ~ StoryPass(s)

(* 自分の変えた項目が承認済でないことがある（StoryPass の 1 つ目の条件が効く場面） *)
NeverOwnChangeUnagreed ==
    \A s \in Stories, i \in Items : ~ (s \in changedBy[i] /\ ~ Agreed(i))

(* 中身を変えた後で、スコープ all のチェックが通ることがある *)
NeverAllPassAfterChange ==
    (\E i \in Items : changedBy[i] # {}) => ~ AllPass

(* 同じ中身に、2 つのストーリーが別々に承認することがある *)
NeverTwoStoriesAgreeSame ==
    ~ \E a, b \in agreements : a.item = b.item /\ a.fp = b.fp /\ a.story # b.story

(* RV-14: 今の中身にほかのストーリーが承認していて承認済なのに、s の承認は失効している *)
NeverBrokenWhileAgreed ==
    \A s \in Stories, i \in Items : ~ (i \in Broken(s) /\ Agreed(i))

(* OnlyOwnAgreementRepairs の 1 つ目の前件: s の失効した承認の項目に、ほかのストーリーが承認する *)
NeverOtherAgreesOnBroken ==
    [][~ \E s, t \in Stories, i \in Items : t # s /\ i \in Broken(s) /\ Agree(t, i)]_vars

(* OnlyOwnAgreementRepairs の 2 つ目の前件: s が、失効した承認の項目を承認し直す *)
NeverRepair ==
    [][~ \E s \in Stories, i \in Items : i \in Broken(s) /\ Agree(s, i)]_vars

(* OwnChangeBreaks の前件: 前に承認したストーリーが、自分でその項目を変える *)
NeverOwnChangeAfterAgree ==
    [][~ \E s \in Stories, i \in Items, f \in Fps : Change(s, i, f) /\ \E a \in agreements : a.item = i /\ a.story = s]_vars

(* 自分しか変えていない項目でも、承認が失効していることがある（誰が変えたかを問わない） *)
NeverBrokenByOwnChangeOnly ==
    \A s \in Stories, i \in Items : ~ (i \in Broken(s) /\ changedBy[i] = {s})
(* DeletedBlocksWithoutAgreement の 1 つ目の前件: 承認したことのある項目が、消したことへの承認のないまま消えている *)
NeverDeletedWithoutAgreement ==
    \A i \in Items : ~ (~ Present(i) /\ EverAgreed(i) /\ ~ HasRecord(i, Absent))

(* 承認したことのある項目を消した後で、消したことへの承認により、スコープ all のチェックが通ることがある *)
NeverAllPassAfterDelete ==
    (\E i \in Items : ~ Present(i) /\ EverAgreed(i)) => ~ AllPass

(* CopyIsNotAgreed の場面: 承認済の項目と同じ中身の別の項目（写し）が、提案中 *)
NeverCopyOfAgreedProposed ==
    ~ \E i, j \in Items : i # j /\ Present(j) /\ content[i] = content[j] /\ Agreed(i) /\ ~ Agreed(j)

(* RenameNeedsAgreement の前件が空振りしない: 承認済のファイルの名前を変える *)
NeverRenameAgreed ==
    [][~ \E s \in Stories, i, j \in Items : Rename(s, i, j) /\ Agreed(i)]_vars
(* StoryPassOnlyWhenSeen の前件が空振りしない: 比べる元と今で違う項目があっても、スコープ story（s）のチェックが通ることがある *)
NeverStoryPassWithDiff ==
    \A s \in Stories : Diff # {} => ~ StoryPass(s)

(* 比べる元にあって今ない項目があっても、消したことへの承認で、スコープ story（s）のチェックが通ることがある *)
NeverStoryPassAfterDelete ==
    ~ \E s \in Stories, i \in Items : base[i] # Absent /\ ~ Present(i) /\ StoryPass(s)

(* 比べる元になく、足して承認して消した項目は、消したことへの承認がなくても、スコープ story（s）のチェックでは見ない（スコープ all では止まる） *)
NeverAddedThenDeletedIgnored ==
    ~ \E s \in Stories, i \in Items :
        base[i] = Absent /\ ~ Present(i) /\ EverAgreed(i) /\ ~ HasRecord(i, Absent) /\ StoryPass(s)

(* s が消したことだけを承認した項目が、承認のない中身で足し直されても、s の失効した承認にならない *)
NeverReaddedAfterDeletionAgreed ==
    ~ \E s \in Stories, i \in Items :
        /\ Present(i) /\ ~ Agreed(i)
        /\ [item |-> i, fp |-> Absent, story |-> s] \in agreements
        /\ ~ \E a \in agreements : a.item = i /\ a.story = s /\ a.fp # Absent
=============================================================================
