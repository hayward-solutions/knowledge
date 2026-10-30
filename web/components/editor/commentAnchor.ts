import { Editor } from "@tiptap/core";
import type { PMDoc } from "@/lib/api/types";
import { htmlToInitialContent } from "@/lib/content/initial";
import { createExtensions } from "./extensions";
import type { EditorChange, EditorValue } from "./types";

function headless(value: EditorValue): Editor {
  return new Editor({
    extensions: createExtensions({
      variant: "compact",
      placeholder: "",
      searchUsers: async () => [],
      searchPages: async () => [],
      upload: { getUpload: () => undefined, onError: () => undefined },
      openLink: () => undefined,
      pickImage: () => undefined,
    }),
    content: htmlToInitialContent(value),
    editable: true,
  });
}

function snapshot(editor: Editor): EditorChange {
  return {
    json: editor.getJSON() as PMDoc,
    html: editor.getHTML(),
    text: editor.getText({ blockSeparator: "\n" }),
  };
}

function findText(editor: Editor, quoted: string): { from: number; to: number } | null {
  if (!quoted) return null;
  let found: { from: number; to: number } | null = null;
  editor.state.doc.descendants((node, pos) => {
    if (found) return false;
    if (!node.isTextblock) return true;
    let text = "";
    const positions: number[] = [];
    node.descendants((child, childPos) => {
      if (child.isText && child.text) {
        for (let i = 0; i < child.text.length; i++) {
          positions.push(pos + 1 + childPos + i);
        }
        text += child.text;
      } else if (child.isLeaf) {
        positions.push(-1);
        text += "\n";
      }
      return true;
    });
    const idx = text.indexOf(quoted);
    if (idx >= 0) {
      const start = positions[idx];
      const end = positions[idx + quoted.length - 1];
      if (start >= 0 && end >= 0) found = { from: start, to: end + 1 };
    }
    return false;
  });
  return found;
}

export function anchorCommentInContent(
  value: EditorValue,
  quotedText: string,
  commentId: string
): EditorChange | null {
  const editor = headless(value);
  try {
    const range = findText(editor, quotedText);
    if (!range) return null;
    editor.chain().setTextSelection(range).setCommentMark(commentId).run();
    return snapshot(editor);
  } finally {
    editor.destroy();
  }
}

export function removeCommentFromContent(
  value: EditorValue,
  commentId: string
): EditorChange | null {
  const editor = headless(value);
  try {
    const removed = editor.commands.unsetCommentMark(commentId);
    return removed ? snapshot(editor) : null;
  } finally {
    editor.destroy();
  }
}
