import { normalizeQuote } from "./commentUtils";

export interface AnchorThread {
  id: string;
  quote: string;
}

const BLOCK_SELECTOR =
  "p, li, h1, h2, h3, h4, h5, h6, td, th, blockquote, summary, figcaption, dt, dd, div";
const SKIP_SELECTOR = 'pre, [data-type="toc"], .kn-toc, button, script, style';
const MARK_SELECTOR = 'span[data-type="comment"]';

interface CharRef {
  node: Text;
  offset: number;
}

interface Corpus {
  text: string;
  chars: CharRef[];
  nodes: Text[];
}

function buildCorpus(root: HTMLElement): Corpus {
  const doc = root.ownerDocument;
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (node.nodeType === Node.TEXT_NODE) return NodeFilter.FILTER_ACCEPT;
      const el = node as Element;
      if (el.matches(SKIP_SELECTOR)) return NodeFilter.FILTER_REJECT;
      if (el.tagName === "BR") return NodeFilter.FILTER_ACCEPT;
      return NodeFilter.FILTER_SKIP;
    },
  });

  let text = "";
  const chars: CharRef[] = [];
  const nodes: Text[] = [];
  let lastWasSpace = true;
  let previousBlock: Element | null = null;
  let pendingBreak = false;
  let lastNode: Text | null = null;

  while (walker.nextNode()) {
    const current = walker.currentNode;
    if (current.nodeType === Node.ELEMENT_NODE) {
      pendingBreak = true;
      continue;
    }
    const node = current as Text;
    if (node.data.length === 0) continue;
    const block = node.parentElement?.closest(BLOCK_SELECTOR) ?? null;
    const boundary = pendingBreak || (previousBlock !== null && block !== previousBlock);
    if (boundary && !lastWasSpace && lastNode) {
      text += " ";
      chars.push({ node: lastNode, offset: lastNode.data.length });
      lastWasSpace = true;
    }
    pendingBreak = false;
    previousBlock = block;
    nodes.push(node);
    lastNode = node;
    const value = node.data;
    for (let i = 0; i < value.length; i += 1) {
      if (/\s/.test(value[i])) {
        if (lastWasSpace) continue;
        text += " ";
        chars.push({ node, offset: i });
        lastWasSpace = true;
      } else {
        text += value[i];
        chars.push({ node, offset: i });
        lastWasSpace = false;
      }
    }
  }
  return { text, chars, nodes };
}

function wrapSegment(node: Text, id: string): void {
  const parent = node.parentNode;
  if (!parent) return;
  const span = node.ownerDocument.createElement("span");
  span.className = "kn-comment-mark";
  span.setAttribute("data-type", "comment");
  span.setAttribute("data-id", id);
  span.setAttribute("data-client-anchor", "true");
  parent.insertBefore(span, node);
  span.appendChild(node);
}

export function anchorQuote(root: HTMLElement, id: string, rawQuote: string): boolean {
  const quote = normalizeQuote(rawQuote);
  if (!quote) return false;
  const corpus = buildCorpus(root);
  const index = corpus.text.indexOf(quote);
  if (index < 0) return false;
  const start = corpus.chars[index];
  const end = corpus.chars[index + quote.length - 1];
  if (!start || !end) return false;
  const first = corpus.nodes.indexOf(start.node);
  const last = corpus.nodes.indexOf(end.node);
  if (first < 0 || last < first) return false;

  for (let i = first; i <= last; i += 1) {
    const node = corpus.nodes[i];
    const from = node === start.node ? start.offset : 0;
    const to = node === end.node ? end.offset + 1 : node.data.length;
    if (to <= from) continue;
    const segment = node.data.slice(from, to);
    if (segment.trim() === "") continue;
    let target = node;
    if (from > 0) target = node.splitText(from);
    if (to - from < target.data.length) target.splitText(to - from);
    wrapSegment(target, id);
  }
  return true;
}

export function unwrapClientAnchors(root: HTMLElement, keep: Set<string>): void {
  root.querySelectorAll<HTMLElement>("span[data-client-anchor]").forEach((span) => {
    const id = span.getAttribute("data-id") ?? "";
    if (keep.has(id)) return;
    const parent = span.parentNode;
    if (!parent) return;
    while (span.firstChild) parent.insertBefore(span.firstChild, span);
    parent.removeChild(span);
    parent.normalize();
  });
}

function hasMark(root: HTMLElement, id: string): boolean {
  return root.querySelector(`${MARK_SELECTOR}[data-id="${CSS.escape(id)}"]`) !== null;
}

export function syncAnchors(root: HTMLElement, threads: AnchorThread[]): Set<string> {
  const wanted = new Set(threads.map((t) => t.id));
  unwrapClientAnchors(root, wanted);
  const found = new Set<string>();
  for (const thread of threads) {
    if (hasMark(root, thread.id) || anchorQuote(root, thread.id, thread.quote)) {
      found.add(thread.id);
    }
  }
  return found;
}

export function applyActiveMark(root: HTMLElement, id: string | null): void {
  root.querySelectorAll<HTMLElement>(MARK_SELECTOR).forEach((span) => {
    span.classList.toggle("is-active", id !== null && span.getAttribute("data-id") === id);
  });
}

export function findMarks(root: HTMLElement, id: string): HTMLElement[] {
  return Array.from(
    root.querySelectorAll<HTMLElement>(`${MARK_SELECTOR}[data-id="${CSS.escape(id)}"]`)
  );
}

export function scrollMarkIntoView(root: HTMLElement, id: string): void {
  const marks = findMarks(root, id);
  if (marks.length === 0) return;
  marks[0].scrollIntoView({ behavior: "smooth", block: "center" });
}
