"use client";

import type { Editor } from "@tiptap/core";
import { useEditorState } from "@tiptap/react";
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  Bold,
  ChevronDown,
  ChevronsUpDown,
  Code,
  Code2,
  Highlighter,
  Image as ImageIcon,
  Italic,
  Link as LinkIcon,
  List,
  ListChecks,
  ListOrdered,
  ListTree,
  Minus,
  Quote,
  Redo2,
  RemoveFormatting,
  Strikethrough,
  Table as TableIcon,
  Tag,
  Underline as UnderlineIcon,
  Undo2,
} from "lucide-react";
import { Menu, MenuItem } from "@/components/ui/Dropdown";
import { PANEL_COLORS, type PanelColor } from "./extensions";
import { ToolbarButton, ToolbarSeparator } from "./ToolbarButton";

const BLOCK_TYPES = [
  { value: "paragraph", label: "Paragraph" },
  { value: "h1", label: "Heading 1" },
  { value: "h2", label: "Heading 2" },
  { value: "h3", label: "Heading 3" },
  { value: "h4", label: "Heading 4" },
];

const PANEL_LABELS: Record<PanelColor, string> = {
  info: "Info panel",
  note: "Note panel",
  success: "Success panel",
  warning: "Warning panel",
  error: "Error panel",
};

function blockTypeOf(editor: Editor): string {
  for (let level = 1; level <= 4; level++) {
    if (editor.isActive("heading", { level })) return `h${level}`;
  }
  if (editor.isActive("heading")) return "other";
  return "paragraph";
}

function alignOf(editor: Editor): string {
  for (const a of ["center", "right", "justify"]) {
    if (editor.isActive({ textAlign: a })) return a;
  }
  return "left";
}

export function Toolbar({
  editor,
  onOpenLink,
  onPickImage,
  canUpload,
}: {
  editor: Editor;
  onOpenLink: () => void;
  onPickImage: () => void;
  canUpload: boolean;
}) {
  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      canUndo: e.can().undo(),
      canRedo: e.can().redo(),
      block: blockTypeOf(e),
      align: alignOf(e),
      bold: e.isActive("bold"),
      italic: e.isActive("italic"),
      underline: e.isActive("underline"),
      strike: e.isActive("strike"),
      code: e.isActive("code"),
      highlight: e.isActive("highlight"),
      link: e.isActive("link"),
      bullet: e.isActive("bulletList"),
      ordered: e.isActive("orderedList"),
      task: e.isActive("taskList"),
      table: e.isActive("table"),
      codeBlock: e.isActive("codeBlock"),
      quote: e.isActive("blockquote"),
      panel: e.isActive("panel"),
    }),
  });

  const chain = () => editor.chain().focus();

  return (
    <div className="kn-toolbar" role="toolbar" aria-label="Formatting">
      <ToolbarButton label="Undo" disabled={!s.canUndo} onClick={() => chain().undo().run()}>
        <Undo2 size={16} />
      </ToolbarButton>
      <ToolbarButton label="Redo" disabled={!s.canRedo} onClick={() => chain().redo().run()}>
        <Redo2 size={16} />
      </ToolbarButton>
      <ToolbarSeparator />
      <select
        className="kn-tb-select"
        aria-label="Block type"
        value={s.block}
        onChange={(e) => {
          const v = e.target.value;
          if (v === "paragraph") chain().setParagraph().run();
          else if (v.startsWith("h"))
            chain()
              .setHeading({ level: Number(v.slice(1)) as 1 | 2 | 3 | 4 })
              .run();
        }}
      >
        {s.block === "other" && <option value="other">Heading</option>}
        {BLOCK_TYPES.map((b) => (
          <option key={b.value} value={b.value}>
            {b.label}
          </option>
        ))}
      </select>
      <ToolbarSeparator />
      <ToolbarButton label="Bold" active={s.bold} onClick={() => chain().toggleBold().run()}>
        <Bold size={16} />
      </ToolbarButton>
      <ToolbarButton label="Italic" active={s.italic} onClick={() => chain().toggleItalic().run()}>
        <Italic size={16} />
      </ToolbarButton>
      <ToolbarButton
        label="Underline"
        active={s.underline}
        onClick={() => chain().toggleUnderline().run()}
      >
        <UnderlineIcon size={16} />
      </ToolbarButton>
      <ToolbarButton label="Strikethrough" active={s.strike} onClick={() => chain().toggleStrike().run()}>
        <Strikethrough size={16} />
      </ToolbarButton>
      <ToolbarButton label="Inline code" active={s.code} onClick={() => chain().toggleCode().run()}>
        <Code size={16} />
      </ToolbarButton>
      <ToolbarButton
        label="Highlight"
        active={s.highlight}
        onClick={() => chain().toggleHighlight().run()}
      >
        <Highlighter size={16} />
      </ToolbarButton>
      <ToolbarSeparator />
      <ToolbarButton label="Link (Ctrl/Cmd+K)" active={s.link} onClick={onOpenLink}>
        <LinkIcon size={16} />
      </ToolbarButton>
      <ToolbarSeparator />
      <ToolbarButton
        label="Bulleted list"
        active={s.bullet}
        onClick={() => chain().toggleBulletList().run()}
      >
        <List size={16} />
      </ToolbarButton>
      <ToolbarButton
        label="Numbered list"
        active={s.ordered}
        onClick={() => chain().toggleOrderedList().run()}
      >
        <ListOrdered size={16} />
      </ToolbarButton>
      <ToolbarButton label="Task list" active={s.task} onClick={() => chain().toggleTaskList().run()}>
        <ListChecks size={16} />
      </ToolbarButton>
      <ToolbarSeparator />
      <ToolbarButton
        label="Align left"
        active={s.align === "left"}
        onClick={() => chain().setTextAlign("left").run()}
      >
        <AlignLeft size={16} />
      </ToolbarButton>
      <ToolbarButton
        label="Align center"
        active={s.align === "center"}
        onClick={() => chain().setTextAlign("center").run()}
      >
        <AlignCenter size={16} />
      </ToolbarButton>
      <ToolbarButton
        label="Align right"
        active={s.align === "right"}
        onClick={() => chain().setTextAlign("right").run()}
      >
        <AlignRight size={16} />
      </ToolbarButton>
      <ToolbarSeparator />
      <ToolbarButton
        label="Insert table"
        active={s.table}
        disabled={s.table}
        onClick={() => chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()}
      >
        <TableIcon size={16} />
      </ToolbarButton>
      {canUpload && (
        <ToolbarButton label="Insert image" onClick={onPickImage}>
          <ImageIcon size={16} />
        </ToolbarButton>
      )}
      <ToolbarButton
        label="Code block"
        active={s.codeBlock}
        onClick={() => chain().toggleCodeBlock().run()}
      >
        <Code2 size={16} />
      </ToolbarButton>
      <ToolbarButton label="Quote" active={s.quote} onClick={() => chain().toggleBlockquote().run()}>
        <Quote size={16} />
      </ToolbarButton>
      <Menu
        align="left"
        width={180}
        label="Panel"
        triggerClassName={`kn-tb-btn kn-tb-menu${s.panel ? " is-active" : ""}`}
        trigger={() => (
          <>
            <span className="kn-tb-text">Panel</span>
            <ChevronDown size={12} />
          </>
        )}
      >
        {(close) => (
          <>
            {PANEL_COLORS.map((color) => (
              <MenuItem
                key={color}
                onClick={() => {
                  chain().togglePanel(color).run();
                  close();
                }}
              >
                <span className={`kn-panel-dot kn-panel-dot-${color}`} />
                {PANEL_LABELS[color]}
              </MenuItem>
            ))}
          </>
        )}
      </Menu>
      <ToolbarButton label="Expand section" onClick={() => chain().insertExpand().run()}>
        <ChevronsUpDown size={16} />
      </ToolbarButton>
      <ToolbarButton label="Status" onClick={() => chain().insertStatus().run()}>
        <Tag size={16} />
      </ToolbarButton>
      <ToolbarButton label="Table of contents" onClick={() => chain().insertToc().run()}>
        <ListTree size={16} />
      </ToolbarButton>
      <ToolbarButton label="Divider" onClick={() => chain().setHorizontalRule().run()}>
        <Minus size={16} />
      </ToolbarButton>
      <ToolbarSeparator />
      <ToolbarButton
        label="Clear formatting"
        onClick={() =>
          chain()
            .unsetBold()
            .unsetItalic()
            .unsetUnderline()
            .unsetStrike()
            .unsetCode()
            .unsetHighlight()
            .unsetLink()
            .unsetTextAlign()
            .run()
        }
      >
        <RemoveFormatting size={16} />
      </ToolbarButton>
    </div>
  );
}
