import { Extension, type Editor } from "@tiptap/core";
import { Plugin, PluginKey, type EditorState } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import type { UploadImageFn } from "../types";

interface UploadAction {
  add?: { id: object; pos: number; name: string };
  remove?: { id: object };
}

const uploadKey = new PluginKey<DecorationSet>("knImageUpload");

export interface UploadHooks {
  getUpload: () => UploadImageFn | undefined;
  onError: (message: string) => void;
}

function findPlaceholder(state: EditorState, id: object): number | null {
  const set = uploadKey.getState(state);
  if (!set) return null;
  const found = set.find(undefined, undefined, (spec) => spec.id === id);
  return found.length ? found[0].from : null;
}

function isImage(file: File): boolean {
  return file.type.startsWith("image/");
}

export function uploadImageFiles(
  editor: Editor,
  files: File[],
  hooks: UploadHooks,
  position?: number
): boolean {
  const upload = hooks.getUpload();
  const images = files.filter(isImage);
  if (!upload || images.length === 0) return false;
  let pos = position ?? editor.state.selection.from;
  for (const file of images) {
    const id = {};
    const action: UploadAction = { add: { id, pos, name: file.name } };
    editor.view.dispatch(editor.state.tr.setMeta(uploadKey, action));
    upload(file)
      .then((result) => {
        const at = findPlaceholder(editor.state, id);
        if (at == null || editor.isDestroyed) return;
        editor.view.dispatch(editor.state.tr.setMeta(uploadKey, { remove: { id } }));
        editor
          .chain()
          .focus()
          .insertContentAt(at, {
            type: "image",
            attrs: { src: result.url, alt: file.name, attachmentId: result.id },
          })
          .run();
      })
      .catch((err: unknown) => {
        if (!editor.isDestroyed) {
          editor.view.dispatch(editor.state.tr.setMeta(uploadKey, { remove: { id } }));
        }
        hooks.onError(
          err instanceof Error && err.message
            ? `Could not upload ${file.name}: ${err.message}`
            : `Could not upload ${file.name}`
        );
      });
    pos = Math.min(pos + 1, editor.state.doc.content.size);
  }
  return true;
}

export const ImageUpload = Extension.create<UploadHooks>({
  name: "imageUpload",

  addOptions() {
    return { getUpload: () => undefined, onError: () => undefined };
  },

  addProseMirrorPlugins() {
    const editor = this.editor;
    const hooks = this.options;
    return [
      new Plugin<DecorationSet>({
        key: uploadKey,
        state: {
          init: () => DecorationSet.empty,
          apply(tr, set) {
            let next = set.map(tr.mapping, tr.doc);
            const action = tr.getMeta(uploadKey) as UploadAction | undefined;
            if (action?.add) {
              const el = document.createElement("div");
              el.className = "kn-upload-placeholder";
              el.setAttribute("contenteditable", "false");
              const spinner = document.createElement("span");
              spinner.className = "kn-upload-spinner";
              const label = document.createElement("span");
              label.textContent = `Uploading ${action.add.name}`;
              el.append(spinner, label);
              next = next.add(tr.doc, [
                Decoration.widget(action.add.pos, el, { id: action.add.id }),
              ]);
            } else if (action?.remove) {
              const id = action.remove.id;
              next = next.remove(next.find(undefined, undefined, (spec) => spec.id === id));
            }
            return next;
          },
        },
        props: {
          decorations(state) {
            return uploadKey.getState(state);
          },
          handlePaste(_view, event) {
            const files = Array.from(event.clipboardData?.files ?? []);
            if (!files.some(isImage) || !hooks.getUpload()) return false;
            event.preventDefault();
            return uploadImageFiles(editor, files, hooks);
          },
          handleDrop(view, event, _slice, moved) {
            if (moved) return false;
            const files = Array.from(event.dataTransfer?.files ?? []);
            if (!files.some(isImage) || !hooks.getUpload()) return false;
            event.preventDefault();
            const at = view.posAtCoords({ left: event.clientX, top: event.clientY });
            return uploadImageFiles(editor, files, hooks, at?.pos);
          },
        },
      }),
    ];
  },
});
