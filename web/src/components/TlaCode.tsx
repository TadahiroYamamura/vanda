import { CodeBlock } from "./Code";

// TlaCode は、TLA+ を色付けし、数学の記号で見せ、コピーしたときはリポジトリの文字のまま（ASCII）にする（CL-07）。
export function TlaCode(props: { source: string }) {
  return <CodeBlock source={props.source} lang="tlaplus" />;
}
