import { Mark, mergeAttributes } from "@tiptap/core";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    commentMark: {
      setCommentMark: (id: string) => ReturnType;
      unsetCommentMark: (id: string) => ReturnType;
    };
  }
}

export const CommentMark = Mark.create({
  name: "comment",
  inclusive: false,
  excludes: "",
  spanning: true,

  addAttributes() {
    return {
      id: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute("data-id"),
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.id ? { "data-id": attributes.id as string } : {},
      },
    };
  },

  parseHTML() {
    return [{ tag: 'span[data-type="comment"]' }];
  },

  renderHTML({ HTMLAttributes }) {
    return ["span", mergeAttributes({ "data-type": "comment" }, HTMLAttributes), 0];
  },

  addCommands() {
    return {
      setCommentMark:
        (id: string) =>
        ({ commands }) =>
          commands.setMark(this.name, { id }),
      unsetCommentMark:
        (id: string) =>
        ({ tr, state, dispatch }) => {
          const type = state.schema.marks[this.name];
          let changed = false;
          state.doc.descendants((node, pos) => {
            if (!node.isInline) return;
            const mark = node.marks.find((m) => m.type === type && m.attrs.id === id);
            if (mark) {
              tr.removeMark(pos, pos + node.nodeSize, mark);
              changed = true;
            }
          });
          if (changed && dispatch) dispatch(tr);
          return changed;
        },
    };
  },
});
