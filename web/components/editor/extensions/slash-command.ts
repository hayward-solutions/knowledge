import { Extension } from "@tiptap/core";
import { PluginKey } from "@tiptap/pm/state";
import Suggestion from "@tiptap/suggestion";
import { createSuggestionRender } from "../suggestion-render";
import { filterSlashItems, toListItem, type SlashItem } from "../slash-items";

export interface SlashCommandOptions {
  hasUpload: () => boolean;
  pickImage: () => void;
}

export const SlashCommand = Extension.create<SlashCommandOptions>({
  name: "slashCommand",

  addOptions() {
    return { hasUpload: () => false, pickImage: () => undefined };
  },

  addProseMirrorPlugins() {
    const options = this.options;
    return [
      Suggestion<SlashItem, SlashItem>({
        editor: this.editor,
        pluginKey: new PluginKey("knSlashCommand"),
        char: "/",
        startOfLine: true,
        allowedPrefixes: null,
        allow: ({ state, range }) => {
          const parent = state.doc.resolve(range.from).parent;
          return parent.type.name === "paragraph";
        },
        items: ({ query }) => filterSlashItems(query, options.hasUpload()),
        command: ({ editor, range, props }) =>
          props.run({ editor, range, pickImage: options.pickImage }),
        render: createSuggestionRender<SlashItem>({
          toItem: toListItem,
          hideWhenEmpty: true,
          emptyLabel: () => "No matching commands",
        }),
      }),
    ];
  },
});
