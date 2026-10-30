"use client";

import { isNodeSelection, type Editor } from "@tiptap/core";
import { NodeSelection } from "@tiptap/pm/state";
import { CellSelection } from "@tiptap/pm/tables";
import { useEditorState } from "@tiptap/react";
import {
  BetweenHorizontalEnd,
  BetweenHorizontalStart,
  BetweenVerticalEnd,
  BetweenVerticalStart,
  Bold,
  Code,
  ExternalLink,
  Highlighter,
  Italic,
  Link as LinkIcon,
  Link2Off,
  MessageSquarePlus,
  Pencil,
  Strikethrough,
  Trash2,
  Underline as UnderlineIcon,
} from "lucide-react";
import { useEffect, useState } from "react";
import { cn } from "@/lib/cn";
import { CODE_LANGUAGES } from "./lowlight";
import { PANEL_COLORS, STATUS_COLORS } from "./extensions";
import { SafeBubbleMenu as BubbleMenu } from "./SafeBubbleMenu";
import { ToolbarButton, ToolbarSeparator } from "./ToolbarButton";
import type { CommentSelection } from "./types";

const TIPPY = {
  duration: 80,
  animation: false,
  theme: "knowledge",
  maxWidth: "none",
  placement: "top" as const,
};

type ShowArgs = Parameters<NonNullable<React.ComponentProps<typeof BubbleMenu>["shouldShow"]>>[0];

function hasFocus({ view, element }: ShowArgs): boolean {
  return view.hasFocus() || element.contains(document.activeElement);
}

function findAncestorDom(editor: Editor, name: string): HTMLElement | null {
  const { $from } = editor.state.selection;
  for (let d = $from.depth; d > 0; d--) {
    if ($from.node(d).type.name === name) {
      const dom = editor.view.nodeDOM($from.before(d));
      return dom instanceof HTMLElement ? dom : null;
    }
  }
  return null;
}

function ancestorRect(editor: Editor, name: string): DOMRect {
  const dom = findAncestorDom(editor, name);
  return dom ? dom.getBoundingClientRect() : new DOMRect(0, 0, 0, 0);
}

function updateSelectedNode(editor: Editor, attrs: Record<string, unknown>) {
  const sel = editor.state.selection;
  if (!(sel instanceof NodeSelection)) return;
  const pos = sel.from;
  editor
    .chain()
    .command(({ tr }) => {
      tr.setNodeMarkup(pos, undefined, { ...sel.node.attrs, ...attrs });
      tr.setSelection(NodeSelection.create(tr.doc, pos));
      return true;
    })
    .run();
}

export function TextBubble({
  editor,
  onOpenLink,
  onRequestComment,
  allowComment,
}: {
  editor: Editor;
  onOpenLink: () => void;
  onRequestComment?: (selection: CommentSelection) => void;
  allowComment: boolean;
}) {
  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      empty: e.state.selection.empty,
      bold: e.isActive("bold"),
      italic: e.isActive("italic"),
      underline: e.isActive("underline"),
      strike: e.isActive("strike"),
      code: e.isActive("code"),
      highlight: e.isActive("highlight"),
      link: e.isActive("link"),
      href: (e.getAttributes("link").href as string | undefined) ?? "",
    }),
  });
  const chain = () => editor.chain().focus();

  return (
    <BubbleMenu
      editor={editor}
      pluginKey="knTextBubble"
      tippyOptions={TIPPY}
      shouldShow={(args) => {
        const { editor: e, state } = args;
        if (!e.isEditable || !hasFocus(args)) return false;
        const sel = state.selection;
        if (isNodeSelection(sel) || sel instanceof CellSelection) return false;
        if (e.isActive("codeBlock")) return false;
        if (!sel.empty) return state.doc.textBetween(sel.from, sel.to).length > 0;
        return e.isActive("link");
      }}
    >
      <div className="kn-bubble" role="toolbar" aria-label="Text formatting">
        {s.empty ? (
          <>
            <a
              className="kn-bubble-href"
              href={s.href}
              target="_blank"
              rel="noopener noreferrer"
              title={s.href}
            >
              <ExternalLink size={12} />
              <span>{s.href}</span>
            </a>
            <ToolbarButton label="Edit link" onClick={onOpenLink}>
              <Pencil size={14} />
            </ToolbarButton>
            <ToolbarButton
              label="Remove link"
              onClick={() => chain().extendMarkRange("link").unsetLink().run()}
            >
              <Link2Off size={14} />
            </ToolbarButton>
          </>
        ) : (
          <>
            <ToolbarButton label="Bold" active={s.bold} onClick={() => chain().toggleBold().run()}>
              <Bold size={14} />
            </ToolbarButton>
            <ToolbarButton
              label="Italic"
              active={s.italic}
              onClick={() => chain().toggleItalic().run()}
            >
              <Italic size={14} />
            </ToolbarButton>
            <ToolbarButton
              label="Underline"
              active={s.underline}
              onClick={() => chain().toggleUnderline().run()}
            >
              <UnderlineIcon size={14} />
            </ToolbarButton>
            <ToolbarButton
              label="Strikethrough"
              active={s.strike}
              onClick={() => chain().toggleStrike().run()}
            >
              <Strikethrough size={14} />
            </ToolbarButton>
            <ToolbarButton
              label="Inline code"
              active={s.code}
              onClick={() => chain().toggleCode().run()}
            >
              <Code size={14} />
            </ToolbarButton>
            <ToolbarButton
              label="Highlight"
              active={s.highlight}
              onClick={() => chain().toggleHighlight().run()}
            >
              <Highlighter size={14} />
            </ToolbarButton>
            <ToolbarSeparator />
            <ToolbarButton label="Link" active={s.link} onClick={onOpenLink}>
              <LinkIcon size={14} />
            </ToolbarButton>
            {allowComment && onRequestComment && (
              <>
                <ToolbarSeparator />
                <ToolbarButton
                  label="Add comment"
                  onClick={() => {
                    const { from, to } = editor.state.selection;
                    onRequestComment({
                      from,
                      to,
                      text: editor.state.doc.textBetween(from, to, " "),
                    });
                  }}
                >
                  <MessageSquarePlus size={14} />
                </ToolbarButton>
              </>
            )}
          </>
        )}
      </div>
    </BubbleMenu>
  );
}

export function TableBubble({ editor }: { editor: Editor }) {
  const chain = () => editor.chain().focus();
  return (
    <BubbleMenu
      editor={editor}
      pluginKey="knTableBubble"
      tippyOptions={{
        ...TIPPY,
        placement: "top-start",
        getReferenceClientRect: () => ancestorRect(editor, "table"),
      }}
      shouldShow={(args) => {
        const { editor: e, state } = args;
        if (!e.isEditable || !hasFocus(args) || !e.isActive("table")) return false;
        return state.selection.empty || state.selection instanceof CellSelection;
      }}
    >
      <div className="kn-bubble kn-table-bubble" role="toolbar" aria-label="Table controls">
        <ToolbarButton label="Add row above" onClick={() => chain().addRowBefore().run()}>
          <BetweenHorizontalStart size={14} />
        </ToolbarButton>
        <ToolbarButton label="Add row below" onClick={() => chain().addRowAfter().run()}>
          <BetweenHorizontalEnd size={14} />
        </ToolbarButton>
        <ToolbarButton label="Add column left" onClick={() => chain().addColumnBefore().run()}>
          <BetweenVerticalStart size={14} />
        </ToolbarButton>
        <ToolbarButton label="Add column right" onClick={() => chain().addColumnAfter().run()}>
          <BetweenVerticalEnd size={14} />
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          label="Delete row"
          className="kn-tb-text-btn"
          onClick={() => chain().deleteRow().run()}
        >
          Delete row
        </ToolbarButton>
        <ToolbarButton
          label="Delete column"
          className="kn-tb-text-btn"
          onClick={() => chain().deleteColumn().run()}
        >
          Delete column
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          label="Toggle header row"
          className="kn-tb-text-btn"
          onClick={() => chain().toggleHeaderRow().run()}
        >
          Header row
        </ToolbarButton>
        <ToolbarButton
          label="Merge or split cells"
          className="kn-tb-text-btn"
          onClick={() => chain().mergeOrSplit().run()}
        >
          Merge / split
        </ToolbarButton>
        <ToolbarSeparator />
        <ToolbarButton
          label="Delete table"
          className="kn-tb-danger"
          onClick={() => chain().deleteTable().run()}
        >
          <Trash2 size={14} />
        </ToolbarButton>
      </div>
    </BubbleMenu>
  );
}

export function StatusBubble({ editor }: { editor: Editor }) {
  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => {
      const sel = e.state.selection;
      if (sel instanceof NodeSelection && sel.node.type.name === "status") {
        return { text: String(sel.node.attrs.text ?? ""), color: String(sel.node.attrs.color) };
      }
      return { text: "", color: "grey" };
    },
  });
  const [text, setText] = useState(s.text);
  useEffect(() => {
    setText(s.text);
  }, [s.text]);

  return (
    <BubbleMenu
      editor={editor}
      pluginKey="knStatusBubble"
      updateDelay={0}
      tippyOptions={TIPPY}
      shouldShow={(args) => {
        const sel = args.state.selection;
        return (
          args.editor.isEditable &&
          hasFocus(args) &&
          sel instanceof NodeSelection &&
          sel.node.type.name === "status"
        );
      }}
    >
      <div className="kn-bubble" role="group" aria-label="Status">
        <input
          className="kn-bubble-input"
          value={text}
          maxLength={30}
          aria-label="Status text"
          onChange={(e) => {
            setText(e.target.value);
            updateSelectedNode(editor, { text: e.target.value.toUpperCase() });
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === "Escape") {
              e.preventDefault();
              editor.commands.focus();
            }
          }}
        />
        {STATUS_COLORS.map((color) => (
          <button
            key={color}
            type="button"
            title={color}
            aria-label={`Status color ${color}`}
            aria-pressed={s.color === color}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => updateSelectedNode(editor, { color })}
            className={cn("kn-color-dot", `kn-status-dot-${color}`, s.color === color && "is-active")}
          />
        ))}
        <ToolbarButton
          label="Remove status"
          className="kn-tb-danger"
          onClick={() => editor.chain().focus().deleteSelection().run()}
        >
          <Trash2 size={14} />
        </ToolbarButton>
      </div>
    </BubbleMenu>
  );
}

export function PanelBubble({ editor }: { editor: Editor }) {
  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => ({ color: String(e.getAttributes("panel").color ?? "") }),
  });
  return (
    <BubbleMenu
      editor={editor}
      pluginKey="knPanelBubble"
      tippyOptions={{
        ...TIPPY,
        placement: "top-end",
        getReferenceClientRect: () => ancestorRect(editor, "panel"),
      }}
      shouldShow={(args) =>
        args.editor.isEditable &&
        hasFocus(args) &&
        args.state.selection.empty &&
        args.editor.isActive("panel")
      }
    >
      <div className="kn-bubble" role="group" aria-label="Panel">
        {PANEL_COLORS.map((color) => (
          <button
            key={color}
            type="button"
            title={`${color} panel`}
            aria-label={`Panel color ${color}`}
            aria-pressed={s.color === color}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => editor.chain().focus().setPanelColor(color).run()}
            className={cn("kn-color-dot", `kn-panel-dot-${color}`, s.color === color && "is-active")}
          />
        ))}
        <ToolbarSeparator />
        <ToolbarButton
          label="Remove panel"
          className="kn-tb-danger"
          onClick={() => editor.chain().focus().lift("panel").run()}
        >
          <Trash2 size={14} />
        </ToolbarButton>
      </div>
    </BubbleMenu>
  );
}

export function CodeBlockBubble({ editor }: { editor: Editor }) {
  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => ({ language: String(e.getAttributes("codeBlock").language ?? "") }),
  });
  return (
    <BubbleMenu
      editor={editor}
      pluginKey="knCodeBubble"
      tippyOptions={{
        ...TIPPY,
        placement: "top-end",
        getReferenceClientRect: () => ancestorRect(editor, "codeBlock"),
      }}
      shouldShow={(args) =>
        args.editor.isEditable && hasFocus(args) && args.editor.isActive("codeBlock")
      }
    >
      <div className="kn-bubble" role="group" aria-label="Code block">
        <select
          className="kn-tb-select"
          aria-label="Code language"
          value={s.language}
          onChange={(e) =>
            editor
              .chain()
              .focus()
              .updateAttributes("codeBlock", { language: e.target.value || null })
              .run()
          }
        >
          {s.language && !CODE_LANGUAGES.some((l) => l.value === s.language) && (
            <option value={s.language}>{s.language}</option>
          )}
          {CODE_LANGUAGES.map((l) => (
            <option key={l.value} value={l.value}>
              {l.label}
            </option>
          ))}
        </select>
      </div>
    </BubbleMenu>
  );
}

const IMAGE_SIZES = [
  { label: "S", width: "25%" },
  { label: "M", width: "50%" },
  { label: "L", width: "75%" },
  { label: "Full", width: "100%" },
];

export function ImageBubble({ editor }: { editor: Editor }) {
  const s = useEditorState({
    editor,
    selector: ({ editor: e }) => {
      const sel = e.state.selection;
      if (sel instanceof NodeSelection && sel.node.type.name === "image") {
        return { width: String(sel.node.attrs.width ?? ""), alt: String(sel.node.attrs.alt ?? "") };
      }
      return { width: "", alt: "" };
    },
  });
  const [alt, setAlt] = useState(s.alt);
  useEffect(() => {
    setAlt(s.alt);
  }, [s.alt]);

  return (
    <BubbleMenu
      editor={editor}
      pluginKey="knImageBubble"
      updateDelay={0}
      tippyOptions={TIPPY}
      shouldShow={(args) => {
        const sel = args.state.selection;
        return (
          args.editor.isEditable &&
          hasFocus(args) &&
          sel instanceof NodeSelection &&
          sel.node.type.name === "image"
        );
      }}
    >
      <div className="kn-bubble" role="group" aria-label="Image">
        {IMAGE_SIZES.map((size) => (
          <ToolbarButton
            key={size.width}
            label={`Width ${size.width}`}
            active={s.width === size.width}
            className="kn-tb-text-btn"
            onClick={() => updateSelectedNode(editor, { width: size.width })}
          >
            {size.label}
          </ToolbarButton>
        ))}
        <ToolbarButton
          label="Original size"
          active={!s.width}
          className="kn-tb-text-btn"
          onClick={() => updateSelectedNode(editor, { width: null })}
        >
          Auto
        </ToolbarButton>
        <ToolbarSeparator />
        <input
          className="kn-bubble-input"
          value={alt}
          placeholder="Alt text"
          aria-label="Alt text"
          onChange={(e) => {
            setAlt(e.target.value);
            updateSelectedNode(editor, { alt: e.target.value });
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === "Escape") {
              e.preventDefault();
              editor.commands.focus();
            }
          }}
        />
        <ToolbarButton
          label="Remove image"
          className="kn-tb-danger"
          onClick={() => editor.chain().focus().deleteSelection().run()}
        >
          <Trash2 size={14} />
        </ToolbarButton>
      </div>
    </BubbleMenu>
  );
}
