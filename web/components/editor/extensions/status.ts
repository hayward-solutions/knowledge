import { Node, mergeAttributes } from "@tiptap/core";
import { NodeSelection } from "@tiptap/pm/state";

export const STATUS_COLORS = ["grey", "blue", "green", "yellow", "red", "purple"] as const;
export type StatusColor = (typeof STATUS_COLORS)[number];

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    status: {
      insertStatus: (attrs?: { text?: string; color?: StatusColor }) => ReturnType;
      updateStatus: (attrs: { text?: string; color?: StatusColor }) => ReturnType;
    };
  }
}

export const Status = Node.create({
  name: "status",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      text: {
        default: "STATUS",
        parseHTML: (element: HTMLElement) => element.textContent ?? "",
        rendered: false,
      },
      color: {
        default: "grey",
        parseHTML: (element: HTMLElement) => {
          const c = element.getAttribute("data-color");
          return c && (STATUS_COLORS as readonly string[]).includes(c) ? c : "grey";
        },
        renderHTML: (attributes: Record<string, unknown>) => ({
          "data-color": attributes.color as string,
        }),
      },
    };
  },

  parseHTML() {
    return [{ tag: 'span[data-type="status"]' }];
  },

  renderHTML({ node, HTMLAttributes }) {
    return [
      "span",
      mergeAttributes({ "data-type": "status" }, HTMLAttributes),
      String(node.attrs.text ?? ""),
    ];
  },

  renderText({ node }) {
    return String(node.attrs.text ?? "");
  },

  addCommands() {
    return {
      insertStatus:
        (attrs) =>
        ({ chain }) =>
          chain()
            .insertContent({
              type: this.name,
              attrs: { text: attrs?.text ?? "STATUS", color: attrs?.color ?? "grey" },
            })
            .command(({ tr }) => {
              const pos = tr.selection.from - 1;
              const inserted = pos >= 0 ? tr.doc.nodeAt(pos) : null;
              if (inserted?.type.name === this.name) {
                tr.setSelection(NodeSelection.create(tr.doc, pos));
              }
              return true;
            })
            .run(),
      updateStatus:
        (attrs) =>
        ({ commands }) =>
          commands.updateAttributes(this.name, attrs),
    };
  },
});
