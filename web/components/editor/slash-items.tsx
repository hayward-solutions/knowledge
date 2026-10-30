import type { Editor, Range } from "@tiptap/core";
import { format } from "date-fns";
import {
  AtSign,
  Calendar,
  CheckCircle2,
  ChevronsUpDown,
  Code2,
  FileText,
  Heading1,
  Heading2,
  Heading3,
  Image as ImageIcon,
  Info,
  List,
  ListChecks,
  ListOrdered,
  ListTree,
  Minus,
  OctagonAlert,
  Quote,
  StickyNote,
  Table as TableIcon,
  Tag,
  TriangleAlert,
} from "lucide-react";
import type { ReactNode } from "react";
import type { SuggestionListItem } from "./SuggestionList";

export interface SlashRunContext {
  editor: Editor;
  range: Range;
  pickImage: () => void;
}

export interface SlashItem {
  key: string;
  title: string;
  description: string;
  keywords: string[];
  icon: ReactNode;
  requiresUpload?: boolean;
  run: (ctx: SlashRunContext) => void;
}

const ICON = 16;

export const SLASH_ITEMS: SlashItem[] = [
  {
    key: "h1",
    title: "Heading 1",
    description: "Large section heading",
    keywords: ["h1", "title", "heading"],
    icon: <Heading1 size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setNode("heading", { level: 1 }).run(),
  },
  {
    key: "h2",
    title: "Heading 2",
    description: "Medium section heading",
    keywords: ["h2", "subtitle", "heading"],
    icon: <Heading2 size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setNode("heading", { level: 2 }).run(),
  },
  {
    key: "h3",
    title: "Heading 3",
    description: "Small section heading",
    keywords: ["h3", "heading"],
    icon: <Heading3 size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setNode("heading", { level: 3 }).run(),
  },
  {
    key: "bullet",
    title: "Bulleted list",
    description: "Unordered list of items",
    keywords: ["ul", "bullet", "list", "unordered"],
    icon: <List size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).toggleBulletList().run(),
  },
  {
    key: "numbered",
    title: "Numbered list",
    description: "Ordered list of items",
    keywords: ["ol", "number", "list", "ordered"],
    icon: <ListOrdered size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).toggleOrderedList().run(),
  },
  {
    key: "task",
    title: "Task list",
    description: "Checklist with checkboxes",
    keywords: ["todo", "task", "check", "checkbox"],
    icon: <ListChecks size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).toggleTaskList().run(),
  },
  {
    key: "table",
    title: "Table",
    description: "3 by 3 table with a header row",
    keywords: ["table", "grid", "columns"],
    icon: <TableIcon size={ICON} />,
    run: ({ editor, range }) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertTable({ rows: 3, cols: 3, withHeaderRow: true })
        .run(),
  },
  {
    key: "code",
    title: "Code block",
    description: "Monospaced block with highlighting",
    keywords: ["code", "pre", "snippet"],
    icon: <Code2 size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setCodeBlock().run(),
  },
  {
    key: "image",
    title: "Image",
    description: "Upload an image",
    keywords: ["image", "picture", "photo", "upload"],
    icon: <ImageIcon size={ICON} />,
    requiresUpload: true,
    run: ({ editor, range, pickImage }) => {
      editor.chain().focus().deleteRange(range).run();
      pickImage();
    },
  },
  {
    key: "panel-info",
    title: "Info panel",
    description: "Blue callout",
    keywords: ["panel", "callout", "info"],
    icon: <Info size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setPanel("info").run(),
  },
  {
    key: "panel-note",
    title: "Note panel",
    description: "Purple callout",
    keywords: ["panel", "callout", "note"],
    icon: <StickyNote size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setPanel("note").run(),
  },
  {
    key: "panel-success",
    title: "Success panel",
    description: "Green callout",
    keywords: ["panel", "callout", "success", "tip"],
    icon: <CheckCircle2 size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setPanel("success").run(),
  },
  {
    key: "panel-warning",
    title: "Warning panel",
    description: "Yellow callout",
    keywords: ["panel", "callout", "warning", "caution"],
    icon: <TriangleAlert size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setPanel("warning").run(),
  },
  {
    key: "panel-error",
    title: "Error panel",
    description: "Red callout",
    keywords: ["panel", "callout", "error", "danger"],
    icon: <OctagonAlert size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setPanel("error").run(),
  },
  {
    key: "expand",
    title: "Expand",
    description: "Collapsible section",
    keywords: ["expand", "collapse", "details", "accordion", "toggle"],
    icon: <ChevronsUpDown size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).insertExpand().run(),
  },
  {
    key: "status",
    title: "Status",
    description: "Colored status lozenge",
    keywords: ["status", "lozenge", "label", "badge"],
    icon: <Tag size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).insertStatus().run(),
  },
  {
    key: "toc",
    title: "Table of contents",
    description: "Built from the page headings",
    keywords: ["toc", "contents", "outline"],
    icon: <ListTree size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).insertToc().run(),
  },
  {
    key: "hr",
    title: "Divider",
    description: "Horizontal rule",
    keywords: ["hr", "divider", "rule", "line", "separator"],
    icon: <Minus size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setHorizontalRule().run(),
  },
  {
    key: "quote",
    title: "Quote",
    description: "Block quotation",
    keywords: ["quote", "blockquote", "citation"],
    icon: <Quote size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).setBlockquote().run(),
  },
  {
    key: "page-link",
    title: "Link to page",
    description: "Search and link another page",
    keywords: ["page", "link", "reference", "wiki"],
    icon: <FileText size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).insertContent("[[").run(),
  },
  {
    key: "mention",
    title: "Mention",
    description: "Mention a person",
    keywords: ["mention", "person", "user", "at"],
    icon: <AtSign size={ICON} />,
    run: ({ editor, range }) =>
      editor.chain().focus().deleteRange(range).insertContent("@").run(),
  },
  {
    key: "date",
    title: "Date",
    description: "Insert today's date as text",
    keywords: ["date", "today", "calendar"],
    icon: <Calendar size={ICON} />,
    run: ({ editor, range }) =>
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent(format(new Date(), "d MMMM yyyy"))
        .run(),
  },
];

export function filterSlashItems(query: string, hasUpload: boolean): SlashItem[] {
  const q = query.trim().toLowerCase();
  const pool = SLASH_ITEMS.filter((i) => hasUpload || !i.requiresUpload);
  if (!q) return pool;
  const scored: { item: SlashItem; score: number }[] = [];
  for (const item of pool) {
    const title = item.title.toLowerCase();
    let score = -1;
    if (title.startsWith(q)) score = 0;
    else if (item.keywords.some((k) => k.startsWith(q))) score = 1;
    else if (title.includes(q)) score = 2;
    else if (item.keywords.some((k) => k.includes(q))) score = 3;
    if (score >= 0) scored.push({ item, score });
  }
  return scored.sort((a, b) => a.score - b.score).map((s) => s.item);
}

export function toListItem(item: SlashItem): SuggestionListItem<SlashItem> {
  return {
    key: item.key,
    label: item.title,
    description: item.description,
    icon: item.icon,
    data: item,
  };
}
