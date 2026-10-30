import { Node, mergeAttributes } from "@tiptap/core";
import { PluginKey } from "@tiptap/pm/state";
import Suggestion from "@tiptap/suggestion";
import { FileText } from "lucide-react";
import { createElement } from "react";
import type { SearchPagesFn, PageLinkItem } from "../types";
import { pageLinkHref } from "../types";
import { createSuggestionRender, debouncedAsync } from "../suggestion-render";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    pageLink: {
      insertPageLink: (attrs: { pageId: string; title: string; href: string }) => ReturnType;
    };
  }
}

export interface PageLinkOptions {
  search: SearchPagesFn;
}

export const PageLink = Node.create<PageLinkOptions>({
  name: "pageLink",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,
  marks: "",

  addOptions() {
    return { search: async () => [] };
  },

  addAttributes() {
    return {
      pageId: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute("data-page-id"),
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.pageId ? { "data-page-id": attributes.pageId as string } : {},
      },
      title: {
        default: "",
        parseHTML: (element: HTMLElement) => element.textContent ?? "",
        rendered: false,
      },
      href: {
        default: "",
        parseHTML: (element: HTMLElement) => element.getAttribute("href") ?? "",
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.href ? { href: attributes.href as string } : {},
      },
    };
  },

  parseHTML() {
    return [{ tag: 'a[data-type="page-link"]', priority: 1000 }];
  },

  renderHTML({ node, HTMLAttributes }) {
    return [
      "a",
      mergeAttributes({ "data-type": "page-link" }, HTMLAttributes),
      String(node.attrs.title ?? ""),
    ];
  },

  renderText({ node }) {
    return String(node.attrs.title ?? "");
  },

  addCommands() {
    return {
      insertPageLink:
        (attrs) =>
        ({ chain }) =>
          chain()
            .insertContent([{ type: this.name, attrs }, { type: "text", text: " " }])
            .run(),
    };
  },

  addProseMirrorPlugins() {
    const search = debouncedAsync((q: string) => this.options.search(q), 180);
    const name = this.name;
    return [
      Suggestion<PageLinkItem, PageLinkItem>({
        editor: this.editor,
        pluginKey: new PluginKey("knPageLinkSuggest"),
        char: "[[",
        allowSpaces: true,
        allowedPrefixes: null,
        items: ({ query }) => search(query),
        allow: ({ state, range }) => {
          const $from = state.doc.resolve(range.from);
          return !!$from.parent.type.contentMatch.matchType(state.schema.nodes[name]);
        },
        command: ({ editor, range, props }) => {
          editor
            .chain()
            .focus()
            .insertContentAt(range, [
              {
                type: name,
                attrs: { pageId: props.id, title: props.title, href: pageLinkHref(props) },
              },
              { type: "text", text: " " },
            ])
            .run();
        },
        render: createSuggestionRender<PageLinkItem>({
          toItem: (item) => ({
            key: item.id,
            label: item.title,
            description: item.spaceName ?? item.spaceKey,
            icon: createElement(FileText, { size: 16 }),
            data: item,
          }),
          emptyLabel: (q) => (q.trim() ? "No matching pages" : "Type to search pages"),
        }),
      }),
    ];
  },
});
