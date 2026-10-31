// 画面の中の場所を URL に持つ（読み込み直しても、同じ所を開くため）。ルーターの依存を足さないため、自分で読む。
import { useEffect, useState } from "react";

export type Route =
  | { page: "review"; params: URLSearchParams }
  | { page: "claim"; id: string; params: URLSearchParams }
  | { page: "file"; path: string; params: URLSearchParams };

export function parseRoute(loc: Location): Route {
  const params = new URLSearchParams(loc.search);
  const m = /^\/claims\/(.+)$/.exec(loc.pathname);
  if (m) return { page: "claim", id: decodeURIComponent(m[1]), params };
  if (loc.pathname === "/files") return { page: "file", path: params.get("path") ?? "", params };
  return { page: "review", params };
}

export function navigate(url: string, replace = false) {
  if (replace) history.replaceState(null, "", url);
  else history.pushState(null, "", url);
  window.dispatchEvent(new PopStateEvent("popstate"));
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(window.location));
  useEffect(() => {
    const on = () => setRoute(parseRoute(window.location));
    window.addEventListener("popstate", on);
    return () => window.removeEventListener("popstate", on);
  }, []);
  return route;
}

export function Link(props: { to: string; children: React.ReactNode; className?: string }) {
  return (
    <a
      href={props.to}
      className={props.className}
      onClick={(e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        e.preventDefault();
        navigate(props.to);
      }}
    >
      {props.children}
    </a>
  );
}
