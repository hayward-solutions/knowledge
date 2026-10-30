import { Node, mergeAttributes } from "@tiptap/core";
import { liftEmptyBlock } from "@tiptap/pm/commands";

export const PANEL_COLORS = ["info", "note", "success", "warning", "error"] as const;
export type PanelColor = (typeof PANEL_COLORS)[number];

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    panel: {
      setPanel: (color?: PanelColor) => ReturnType;
      togglePanel: (color?: PanelColor) => ReturnType;
      setPanelColor: (color: PanelColor) => ReturnType;
    };
  }
}

export const Panel = Node.create({
  name: "panel",
  group: "block",
  content: "block+",
  defining: true,

  addAttributes() {
    return {
      color: {
        default: "info",
        parseHTML: (element: HTMLElement) => {
          const c = element.getAttribute("data-color");
          return c && (PANEL_COLORS as readonly string[]).includes(c) ? c : "info";
        },
        renderHTML: (attributes: Record<string, unknown>) => ({
          "data-color": attributes.color as string,
        }),
      },
    };
  },

  parseHTML() {
    return [{ tag: 'div[data-type="panel"]' }];
  },

  renderHTML({ HTMLAttributes }) {
    return ["div", mergeAttributes({ "data-type": "panel" }, HTMLAttributes), 0];
  },

  addCommands() {
    return {
      setPanel:
        (color = "info") =>
        ({ commands }) =>
          commands.wrapIn(this.name, { color }),
      togglePanel:
        (color = "info") =>
        ({ commands }) =>
          commands.toggleWrap(this.name, { color }),
      setPanelColor:
        (color) =>
        ({ commands }) =>
          commands.updateAttributes(this.name, { color }),
    };
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
        const panel = $from.node(depth);
        if (panel.childCount < 2 || $from.index(depth) !== panel.childCount - 1) return false;
        return liftEmptyBlock(state, view.dispatch);
      },
    };
  },
});
