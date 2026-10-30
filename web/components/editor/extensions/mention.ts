import type { Editor, Range } from "@tiptap/core";
import Mention from "@tiptap/extension-mention";
import type { UserRef } from "@/lib/api/types";
import { createElement } from "react";
import { Avatar } from "@/components/ui/Avatar";
import { createSuggestionRender, debouncedAsync } from "../suggestion-render";
import type { SearchUsersFn } from "../types";

export const KnMention = Mention.extend({
  addAttributes() {
    return {
      id: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute("data-id"),
        rendered: false,
      },
      label: {
        default: null,
        parseHTML: (element: HTMLElement) => (element.textContent ?? "").replace(/^@/, ""),
        rendered: false,
      },
      mentionSuggestionChar: {
        default: "@",
        rendered: false,
      },
    };
  },

  renderHTML({ node }) {
    return [
      "span",
      { "data-type": "mention", "data-id": String(node.attrs.id ?? "") },
      `@${node.attrs.label ?? ""}`,
    ];
  },

  renderText({ node }) {
    return `@${node.attrs.label ?? ""}`;
  },
});

export function mentionSuggestion(search: SearchUsersFn) {
  const run = debouncedAsync((q: string) => search(q), 180);
  return {
    char: "@",
    items: ({ query }: { query: string }) => run(query),
    render: createSuggestionRender<UserRef>({
      toItem: (user) => ({
        key: user.id,
        label: user.display_name,
        description: user.email,
        icon: createElement(Avatar, { name: user.display_name, seed: user.id, size: 22 }),
        data: user,
      }),
      emptyLabel: (q) => (q.trim() ? "No matching people" : "Type a name"),
    }),
    command: ({
      editor,
      range,
      props,
    }: {
      editor: Editor;
      range: Range;
      props: UserRef;
    }) => {
      const nodeAfter = editor.view.state.selection.$to.nodeAfter;
      if (nodeAfter?.text?.startsWith(" ")) range.to += 1;
      editor
        .chain()
        .focus()
        .insertContentAt(range, [
          {
            type: "mention",
            attrs: { id: props.id, label: props.display_name, mentionSuggestionChar: "@" },
          },
          { type: "text", text: " " },
        ])
        .run();
    },
  };
}
