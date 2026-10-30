"use client";

import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, type MouseEvent } from "react";
import { apiUrl } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { extractHeadings, type HeadingInfo } from "@/lib/content/headings";
import { lowlight } from "./lowlight";

export interface ContentViewProps {
  html: string;
  className?: string;
  onHeadingsReady?: (headings: HeadingInfo[]) => void;
  onCommentMarkClick?: (id: string, element: HTMLElement) => void;
  activeCommentId?: string | null;
}

export function useHeadings(html: string): HeadingInfo[] {
  return useMemo(() => extractHeadings(html), [html]);
}

interface HastNode {
  type: string;
  value?: string;
  tagName?: string;
  properties?: { className?: string[] | string };
  children?: HastNode[];
}

function hastToDom(node: HastNode, doc: Document): Node {
  if (node.type === "text") return doc.createTextNode(node.value ?? "");
  if (node.type === "element") {
    const el = doc.createElement(node.tagName ?? "span");
    const cls = node.properties?.className;
    if (cls) el.className = Array.isArray(cls) ? cls.join(" ") : cls;
    for (const child of node.children ?? []) el.appendChild(hastToDom(child, doc));
    return el;
  }
  const frag = doc.createDocumentFragment();
  for (const child of node.children ?? []) frag.appendChild(hastToDom(child, doc));
  return frag;
}

function languageOf(code: HTMLElement): string | null {
  for (const c of Array.from(code.classList)) {
    if (c.startsWith("language-")) return c.slice("language-".length);
  }
  return null;
}

function decorateHeadings(root: HTMLElement, headings: HeadingInfo[]) {
  const nodes = root.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6");
  nodes.forEach((node, i) => {
    const h = headings[i];
    if (h && node.id !== h.id) node.id = h.id;
  });
}

function buildToc(root: HTMLElement, headings: HeadingInfo[]) {
  const doc = root.ownerDocument;
  root.querySelectorAll<HTMLElement>('div[data-type="toc"]').forEach((host) => {
    host.replaceChildren();
    host.classList.add("kn-toc");
    const title = doc.createElement("div");
    title.className = "kn-toc-title";
    title.textContent = "Contents";
    host.appendChild(title);
    if (headings.length === 0) {
      const empty = doc.createElement("div");
      empty.className = "kn-toc-empty";
      empty.textContent = "No headings on this page";
      host.appendChild(empty);
      return;
    }
    const minLevel = Math.min(...headings.map((h) => h.level));
    const list = doc.createElement("ul");
    for (const h of headings) {
      const li = doc.createElement("li");
      li.className = `kn-toc-l${Math.min(h.level - minLevel + 1, 4)}`;
      const a = doc.createElement("a");
      a.href = `#${h.id}`;
      a.textContent = h.text || "Untitled";
      li.appendChild(a);
      list.appendChild(li);
    }
    host.appendChild(list);
  });
}

function decorateTasks(root: HTMLElement) {
  const doc = root.ownerDocument;
  root.querySelectorAll<HTMLElement>('li[data-type="taskItem"]').forEach((li) => {
    if (li.dataset.knReady === "true") return;
    li.dataset.knReady = "true";
    const checked = li.getAttribute("data-checked") === "true";
    const content = doc.createElement("div");
    while (li.firstChild) content.appendChild(li.firstChild);
    const label = doc.createElement("label");
    label.contentEditable = "false";
    const input = doc.createElement("input");
    input.type = "checkbox";
    input.checked = checked;
    input.disabled = true;
    input.setAttribute("aria-label", checked ? "Completed task" : "Open task");
    label.appendChild(input);
    label.appendChild(doc.createElement("span"));
    li.appendChild(label);
    li.appendChild(content);
  });
}

function decorateCode(root: HTMLElement) {
  const doc = root.ownerDocument;
  root.querySelectorAll<HTMLElement>("pre").forEach((pre) => {
    if (pre.dataset.knReady === "true") return;
    pre.dataset.knReady = "true";
    const code = pre.querySelector<HTMLElement>("code");
    if (!code) return;
    const lang = languageOf(code);
    if (lang && lowlight.registered(lang)) {
      try {
        const tree = lowlight.highlight(lang, code.textContent ?? "");
        code.replaceChildren(hastToDom(tree as unknown as HastNode, doc));
      } catch {
        code.classList.add("kn-no-highlight");
      }
    }
    if (lang) pre.dataset.language = lang;
    const button = doc.createElement("button");
    button.type = "button";
    button.className = "kn-copy";
    button.textContent = "Copy";
    button.setAttribute("aria-label", "Copy code");
    pre.appendChild(button);
  });
}

function decorateTables(root: HTMLElement) {
  const doc = root.ownerDocument;
  root.querySelectorAll<HTMLElement>("table").forEach((table) => {
    if (table.parentElement?.classList.contains("kn-table-wrap")) return;
    const wrap = doc.createElement("div");
    wrap.className = "kn-table-wrap";
    table.parentNode?.insertBefore(wrap, table);
    wrap.appendChild(table);
  });
}

function decorateImages(root: HTMLElement) {
  root.querySelectorAll<HTMLImageElement>("img").forEach((img) => {
    const src = img.getAttribute("src") ?? "";
    if (src.startsWith("/api/")) img.setAttribute("src", apiUrl(src));
    img.loading = "lazy";
  });
}

function decorateComments(root: HTMLElement) {
  root.querySelectorAll<HTMLElement>('span[data-type="comment"]').forEach((span) => {
    span.classList.add("kn-comment-mark");
  });
}

function applyActiveComment(root: HTMLElement, id: string | null | undefined) {
  root.querySelectorAll<HTMLElement>(".kn-comment-mark").forEach((span) => {
    span.classList.toggle("is-active", Boolean(id) && span.getAttribute("data-id") === id);
  });
}

export function ContentView({
  html,
  className,
  onHeadingsReady,
  onCommentMarkClick,
  activeCommentId,
}: ContentViewProps) {
  const ref = useRef<HTMLDivElement>(null);
  const router = useRouter();
  const headings = useHeadings(html);
  const inner = useMemo(() => ({ __html: html }), [html]);
  const readyRef = useRef(onHeadingsReady);

  useEffect(() => {
    readyRef.current = onHeadingsReady;
  });

  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    decorateHeadings(root, headings);
    buildToc(root, headings);
    decorateTasks(root);
    decorateCode(root);
    decorateTables(root);
    decorateImages(root);
    decorateComments(root);
    readyRef.current?.(headings);
  }, [html, headings]);

  useEffect(() => {
    const root = ref.current;
    if (root) applyActiveComment(root, activeCommentId);
  }, [html, activeCommentId]);

  function onClick(event: MouseEvent<HTMLDivElement>) {
    const target = event.target as HTMLElement | null;
    if (!target) return;

    const copy = target.closest<HTMLElement>("button.kn-copy");
    if (copy) {
      const code = copy.parentElement?.querySelector("code")?.textContent ?? "";
      navigator.clipboard
        ?.writeText(code)
        .then(() => {
          copy.textContent = "Copied";
          window.setTimeout(() => {
            copy.textContent = "Copy";
          }, 1500);
        })
        .catch(() => {
          copy.textContent = "Failed";
          window.setTimeout(() => {
            copy.textContent = "Copy";
          }, 1500);
        });
      return;
    }

    const mark = target.closest<HTMLElement>(".kn-comment-mark");
    if (mark && onCommentMarkClick) {
      const id = mark.getAttribute("data-id");
      if (id) {
        event.preventDefault();
        onCommentMarkClick(id, mark);
        return;
      }
    }

    const anchor = target.closest<HTMLAnchorElement>("a[href]");
    if (anchor && !event.defaultPrevented) {
      const href = anchor.getAttribute("href") ?? "";
      const modified = event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
      if (href.startsWith("/") && !href.startsWith("//") && !modified && event.button === 0) {
        if (anchor.target && anchor.target !== "_self") return;
        event.preventDefault();
        router.push(href);
      }
    }
  }

  return (
    <div
      ref={ref}
      className={cn("kn-content", className)}
      onClick={onClick}
      dangerouslySetInnerHTML={inner}
    />
  );
}
