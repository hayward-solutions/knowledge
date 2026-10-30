import { Node } from "@tiptap/core";
import { liftEmptyBlock } from "@tiptap/pm/commands";
import { TextSelection } from "@tiptap/pm/state";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    expand: {
      insertExpand: (title?: string) => ReturnType;
    };
  }
}

export const ExpandSummary = Node.create({
  name: "expandSummary",
  content: "inline*",
  defining: true,
  isolating: true,

  parseHTML() {
    return [{ tag: "summary" }];
  },

  renderHTML() {
    return ["summary", 0];
  },

  addKeyboardShortcuts() {
    return {
      Enter: ({ editor }) => {
        const { state, view } = editor;
        const { $from } = state.selection;
        if ($from.parent.type.name !== this.name) return false;
        const bodyStart = $from.after() + 1;
        const tr = state.tr.setSelection(TextSelection.near(state.doc.resolve(bodyStart), 1));
        view.dispatch(tr.scrollIntoView());
        return true;
      },
    };
  },
});

export const ExpandBody = Node.create({
  name: "expandBody",
  content: "block+",
  defining: true,
  isolating: true,

  parseHTML() {
    return [{ tag: 'details[data-type="expand"] > div' }];
  },

  renderHTML() {
    return ["div", 0];
  },

  addKeyboardShortcuts() {
    return {
      Enter: ({ editor }) => {
        const { state, view } = editor;
        const { $from, empty } = state.selection;
        if (!empty || $from.parent.type.name !== "paragraph") return false;
        if ($from.parent.content.size !== 0) return false;
        const depth = $from.depth - 1;
        if (depth < 0 || $from.node(depth).type.name !== this.name) return false;
        const body = $from.node(depth);
        if (body.childCount < 2 || $from.index(depth) !== body.childCount - 1) return false;
        return liftEmptyBlock(state, view.dispatch);
      },
    };
  },
});

export const Expand = Node.create({
  name: "expand",
  group: "block",
  content: "expandSummary expandBody",
  defining: true,
  isolating: true,

  parseHTML() {
    return [{ tag: 'details[data-type="expand"]' }];
  },

  renderHTML() {
    return ["details", { "data-type": "expand" }, 0];
  },

  addNodeView() {
    return () => {
      const dom = document.createElement("details");
      dom.setAttribute("data-type", "expand");
      dom.open = true;
      dom.addEventListener("click", (event) => {
        const target = event.target as HTMLElement | null;
        const summary = target?.closest("summary");
        if (!summary || summary.parentElement !== dom) return;
        event.preventDefault();
        const rect = summary.getBoundingClientRect();
        if (event.clientX - rect.left <= 28) {
          if (dom.dataset.collapsed === "true") delete dom.dataset.collapsed;
          else dom.dataset.collapsed = "true";
        }
      });
      return {
        dom,
        contentDOM: dom,
        ignoreMutation: (mutation) =>
          mutation.type === "attributes" && mutation.target === dom,
      };
    };
  },

  addCommands() {
    return {
      insertExpand:
        (title = "") =>
        ({ chain, state }) => {
          const start = Math.max(0, state.selection.from - 1);
          return chain()
            .insertContent({
              type: this.name,
              content: [
                {
                  type: "expandSummary",
                  content: title ? [{ type: "text", text: title }] : [],
                },
                { type: "expandBody", content: [{ type: "paragraph" }] },
              ],
            })
            .command(({ tr }) => {
              let found = -1;
              tr.doc.nodesBetween(start, tr.doc.content.size, (node, pos) => {
                if (found >= 0) return false;
                if (node.type.name === this.name) {
                  found = pos;
                  return false;
                }
                return true;
              });
              if (found < 0) return true;
              tr.setSelection(TextSelection.near(tr.doc.resolve(found + 2), 1));
              return true;
            })
            .run();
        },
    };
  },
});
