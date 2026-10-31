import type { ReactNode } from "react";
import { stageLabel, typeLabel } from "../labels";

// ステージの並び（計画→実装→テスト→リリース→運用）。決まった値のほかのステージは、後ろに出てきた順で並べる
const STAGES = ["plan", "implementation", "test", "release", "operation"];

type Heading = "h3" | "h4" | "h5";
const next: Record<Heading, "h4" | "h5" | "h6"> = { h3: "h4", h4: "h5", h5: "h6" };

// EvidenceGroups は、エビデンスをステージの見出しで分け、その下をタイプの見出しで分けて示す（CL-02）。
// ステージとタイプは vanda が決めた値で、1 つのエビデンスにそれぞれ 1 つなので、札で並べるより分けて示す。
// エビデンスのないステージ・タイプの見出しは出さない。タイプの並びは、そのステージの中で出てきた順。各エビデンスの中身は render が作る
export function EvidenceGroups<T extends { stage: string; type: string }>(props: { items: T[]; level: Heading; render: (ev: T, index: number) => ReactNode }) {
  const H = props.level;
  const T = next[props.level];
  const stages = [...STAGES, ...props.items.map((e) => e.stage).filter((s) => !STAGES.includes(s))].filter((s, i, a) => a.indexOf(s) === i);
  return (
    <div className="evidence-groups">
      {stages.map((stage) => {
        const inStage = props.items.map((ev, i) => [ev, i] as const).filter(([ev]) => ev.stage === stage);
        if (inStage.length === 0) return null;
        const types = inStage.map(([ev]) => ev.type).filter((t, i, a) => a.indexOf(t) === i);
        return (
          <section key={stage} className="evidence-stage">
            <H className="evidence-stage-head">{stageLabel[stage] ?? stage}</H>
            {types.map((type) => (
              <div key={type} className="evidence-type-group">
                <T className="evidence-type-head">{typeLabel[type] ?? type}</T>
                {inStage
                  .filter(([ev]) => ev.type === type)
                  .map(([ev, i]) => (
                    <div key={i} className="evidence">
                      {props.render(ev, i)}
                    </div>
                  ))}
              </div>
            ))}
          </section>
        );
      })}
    </div>
  );
}
