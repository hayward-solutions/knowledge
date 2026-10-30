import { mergeAttributes } from "@tiptap/core";
import TaskItem from "@tiptap/extension-task-item";

export const KnTaskItem = TaskItem.extend({
  renderHTML({ node, HTMLAttributes }) {
    return [
      "li",
      mergeAttributes(HTMLAttributes, {
        "data-type": this.name,
        "data-checked": node.attrs.checked ? "true" : "false",
      }),
      0,
    ];
  },
});
