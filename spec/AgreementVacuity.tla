------------------------- MODULE AgreementVacuity -------------------------
(* Agreement の性質が空振りで成り立っていないかを確かめるための、破られるはずの性質。               *)
(* どれも、TLC が反例を見つける（その状態や操作が一度は起きる）ことを、spec/agreement_vacuity_test.go で確かめる。 *)
(* Agreement.tla の NeverChange・NeverAgree・NeverBroken が確かめていない、性質の前提と RV-14 の場面を足す。 *)
EXTENDS Agreement

(* StoryPass が、自分の変えた項目があっても成り立つことがある（BrokenBlocksStory の後件が空振りしない） *)
NeverOwnChangePasses ==
    \A s \in Stories : (\E i \in Items : s \in changedBy[i]) => ~ StoryPass(s)

(* 自分の変えた項目が合意済でないことがある（StoryPass の 1 つ目の条件が効く場面） *)
NeverOwnChangeUnagreed ==
    \A s \in Stories, i \in Items : ~ (s \in changedBy[i] /\ ~ Agreed(i))

(* 中身を変えた後で、全部の範囲の審査が通ることがある *)
NeverAllPassAfterChange ==
    (\E i \in Items : changedBy[i] # {}) => ~ AllPass

(* 同じ中身に、2 つのストーリーが別々に合意することがある *)
NeverTwoStoriesAgreeSame ==
    ~ \E a, b \in agreements : a.item = b.item /\ a.fp = b.fp /\ a.story # b.story

(* RV-14: 今の中身にほかのストーリーが合意していて合意済なのに、s の合意は外れている *)
NeverBrokenWhileAgreed ==
    \A s \in Stories, i \in Items : ~ (i \in Broken(s) /\ Agreed(i))

(* OnlyOwnAgreementRepairs の 1 つ目の前件: s の外れた合意の項目に、ほかのストーリーが合意する *)
NeverOtherAgreesOnBroken ==
    [][~ \E s, t \in Stories, i \in Items : t # s /\ i \in Broken(s) /\ Agree(t, i)]_vars

(* OnlyOwnAgreementRepairs の 2 つ目の前件: s が、外れた合意の項目に合意し直す *)
NeverRepair ==
    [][~ \E s \in Stories, i \in Items : i \in Broken(s) /\ Agree(s, i)]_vars

(* OwnChangeBreaks の前件: 前に合意したストーリーが、自分でその項目を変える *)
NeverOwnChangeAfterAgree ==
    [][~ \E s \in Stories, i \in Items, f \in Fps : Change(s, i, f) /\ \E a \in agreements : a.item = i /\ a.story = s]_vars

(* 自分しか変えていない項目でも、合意が外れていることがある（誰が変えたかを問わない） *)
NeverBrokenByOwnChangeOnly ==
    \A s \in Stories, i \in Items : ~ (i \in Broken(s) /\ changedBy[i] = {s})
=============================================================================
