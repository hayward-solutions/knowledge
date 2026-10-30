import { Extension } from "@tiptap/core";

export interface LinkShortcutOptions {
  open: () => void;
}

export const LinkShortcut = Extension.create<LinkShortcutOptions>({
  name: "linkShortcut",

  addOptions() {
    return { open: () => undefined };
  },

  addKeyboardShortcuts() {
    return {
      "Mod-k": () => {
        this.options.open();
        return true;
      },
    };
  },
});
