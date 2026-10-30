import Heading from "@tiptap/extension-heading";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { HEADING_ID_PATTERN, slugify, uniqueSlug } from "@/lib/content/slugify";

const headingIdKey = new PluginKey("knHeadingIds");

export const HeadingWithId = Heading.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      id: {
        default: null,
        parseHTML: (element: HTMLElement) => {
          const id = element.getAttribute("id");
          return id && HEADING_ID_PATTERN.test(id) ? id : null;
        },
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.id ? { id: attributes.id as string } : {},
      },
    };
  },

  addProseMirrorPlugins() {
    return [
      ...(this.parent?.() ?? []),
      new Plugin({
        key: headingIdKey,
        appendTransaction(transactions, _old, state) {
          if (!transactions.some((t) => t.docChanged)) return null;
          const taken = new Set<string>();
          const updates: { pos: number; id: string }[] = [];
          state.doc.descendants((node, pos) => {
            if (node.type.name !== "heading") return;
            const id = uniqueSlug(slugify(node.textContent), taken);
            if (node.attrs.id !== id) updates.push({ pos, id });
          });
          if (updates.length === 0) return null;
          const tr = state.tr;
          for (const u of updates) {
            const node = state.doc.nodeAt(u.pos);
            if (!node) continue;
            tr.setNodeMarkup(u.pos, undefined, { ...node.attrs, id: u.id });
          }
          tr.setMeta("addToHistory", false);
          return tr;
        },
      }),
    ];
  },
});
