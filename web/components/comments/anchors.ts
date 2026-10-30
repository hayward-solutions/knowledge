import { normalizeQuote } from "./commentUtils";

export interface AnchorThread {
  id: string;
  quote: string;
  prefix?: string;
  suffix?: string;
}

export const CONTEXT_LENGTH = 32;

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
  marked: boolean[];
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
  const marked: boolean[] = [];
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
    marked.push(node.parentElement?.closest(MARK_SELECTOR) != null);
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
  return { text, chars, nodes, marked };
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

function commonSuffixLength(a: string, b: string): number {
  let n = 0;
  while (n < a.length && n < b.length && a[a.length - 1 - n] === b[b.length - 1 - n]) n += 1;
  return n;
}

function commonPrefixLength(a: string, b: string): number {
  let n = 0;
  while (n < a.length && n < b.length && a[n] === b[n]) n += 1;
  return n;
}

interface Candidate {
  index: number;
  score: number;
  clean: boolean;
}

function pickCandidate(
  corpus: Corpus,
  quote: string,
  prefix: string,
  suffix: string
): number {
  const hasContext = prefix !== "" || suffix !== "";
  const candidates: Candidate[] = [];
  let from = 0;
  for (;;) {
    const index = corpus.text.indexOf(quote, from);
    if (index < 0) break;
    from = index + 1;
    const start = corpus.chars[index];
    const end = corpus.chars[index + quote.length - 1];
    if (!start || !end) continue;
    const first = corpus.nodes.indexOf(start.node);
    const last = corpus.nodes.indexOf(end.node);
    if (first < 0 || last < first) continue;
    let clean = true;
    for (let i = first; i <= last; i += 1) {
      if (corpus.marked[i]) {
        clean = false;
        break;
      }
    }
    const score = hasContext
      ? commonSuffixLength(corpus.text.slice(0, index), prefix) +
        commonPrefixLength(corpus.text.slice(index + quote.length), suffix)
      : 0;
    candidates.push({ index, score, clean });
  }
  if (candidates.length === 0) return -1;
  const best = (list: Candidate[]) =>
    list.reduce((a, c) => (c.score > a.score ? c : a), list[0]);
  const clean = candidates.filter((c) => c.clean);
  if (clean.length === 0) return best(candidates).index;
  const bestClean = best(clean);
  if (hasContext) {
    const bestAny = best(candidates);
    if (bestAny.score > bestClean.score) return bestAny.index;
  }
  return bestClean.index;
}

export function anchorQuote(
  root: HTMLElement,
  id: string,
  rawQuote: string,
  rawPrefix = "",
  rawSuffix = ""
): boolean {
  const quote = normalizeQuote(rawQuote);
  if (!quote) return false;
  const corpus = buildCorpus(root);
  const prefix = rawPrefix.replace(/\s+/g, " ");
  const suffix = rawSuffix.replace(/\s+/g, " ");
  const index = pickCandidate(corpus, quote, prefix, suffix);
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
    if (hasMark(root, thread.id) ||
      anchorQuote(root, thread.id, thread.quote, thread.prefix, thread.suffix)) {
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

export function captureContext(
  root: HTMLElement,
  range: Range,
  quote: string
): { prefix: string; suffix: string } | null {
  if (!quote) return null;
  const corpus = buildCorpus(root);
  const count = corpus.chars.length;
  if (count === 0) return null;
  const compare = (i: number) => {
    const ref = corpus.chars[i];
    try {
      return range.comparePoint(ref.node, ref.offset);
    } catch {
      return 1;
    }
  };
  let lo = 0;
  let hi = count;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (compare(mid) < 0) lo = mid + 1;
    else hi = mid;
  }
  let start = lo;
  while (start < count && corpus.text[start] === " ") start += 1;
  if (!corpus.text.startsWith(quote, start)) return null;
  const end = start + quote.length;
  return {
    prefix: corpus.text.slice(Math.max(0, start - CONTEXT_LENGTH), start),
    suffix: corpus.text.slice(end, end + CONTEXT_LENGTH),
  };
}
