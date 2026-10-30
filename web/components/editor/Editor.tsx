"use client";

import type { Editor as TiptapEditor } from "@tiptap/core";
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import type { PMDoc } from "@/lib/api/types";
import { htmlToInitialContent } from "@/lib/content/initial";
import { isEmptyDoc } from "@/lib/content/doc";
import { cn } from "@/lib/cn";
import { useToast } from "@/components/ui/Toast";
import {
  CodeBlockBubble,
  ImageBubble,
  PanelBubble,
  StatusBubble,
  TableBubble,
  TextBubble,
} from "./BubbleMenus";
import { defaultSearchPages, defaultSearchUsers } from "./defaults";
import { createExtensions, uploadImageFiles } from "./extensions";
import {
  LinkPopover,
  computeLinkPopover,
  type LinkPopoverState,
} from "./LinkPopover";
import { Toolbar } from "./Toolbar";
import type {
  CommentSelection,
  EditorChange,
  EditorHandle,
  EditorValue,
  SearchPagesFn,
  SearchUsersFn,
  UploadImageFn,
} from "./types";

export interface EditorProps {
  value: EditorValue;
  onChange?: (change: EditorChange) => void;
  editable?: boolean;
  variant?: "full" | "compact";
  placeholder?: string;
  uploadImage?: UploadImageFn;
  searchUsers?: SearchUsersFn;
  searchPages?: SearchPagesFn;
  onRequestComment?: (selection: CommentSelection) => void;
  autofocus?: boolean | "start" | "end";
  className?: string;
  ariaLabel?: string;
}

const FULL_PLACEHOLDER = "Type / for commands, @ to mention, [[ to link a page";

function snapshot(editor: TiptapEditor): EditorChange {
  return {
    json: editor.getJSON() as PMDoc,
    html: editor.getHTML(),
    text: editor.getText({ blockSeparator: "\n" }),
  };
}

export const Editor = forwardRef<EditorHandle, EditorProps>(function Editor(
  {
    value,
    onChange,
    editable = true,
    variant = "full",
    placeholder,
    uploadImage,
    searchUsers,
    searchPages,
    onRequestComment,
    autofocus = false,
    className,
    ariaLabel,
  },
  ref
) {
  const toast = useToast();
  const isFull = variant === "full";

  const onChangeRef = useRef(onChange);
  const uploadRef = useRef(uploadImage);
  const usersRef = useRef(searchUsers ?? defaultSearchUsers);
  const pagesRef = useRef(searchPages ?? defaultSearchPages);
  const toastRef = useRef(toast);
  const editorRef = useRef<TiptapEditor | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const initialRef = useRef(htmlToInitialContent(value));

  useEffect(() => {
    onChangeRef.current = onChange;
    uploadRef.current = uploadImage;
    usersRef.current = searchUsers ?? defaultSearchUsers;
    pagesRef.current = searchPages ?? defaultSearchPages;
    toastRef.current = toast;
  });

  const [linkUi, setLinkUi] = useState<LinkPopoverState | null>(null);

  const openLink = useCallback(() => {
    const e = editorRef.current;
    if (!e || !e.isEditable) return;
    setLinkUi(computeLinkPopover(e));
  }, []);

  const pickImage = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const extensions = useMemo(
    () =>
      createExtensions({
        variant,
        placeholder: placeholder ?? (isFull ? FULL_PLACEHOLDER : "Write something"),
        searchUsers: (q) => usersRef.current(q),
        searchPages: (q) => pagesRef.current(q),
        upload: {
          getUpload: () => uploadRef.current,
          onError: (message) => toastRef.current.error(message),
        },
        openLink,
        pickImage,
      }),
    [variant, placeholder, isFull, openLink, pickImage]
  );

  const editor = useEditor({
    immediatelyRender: false,
    shouldRerenderOnTransaction: false,
    extensions,
    content: initialRef.current,
    editable,
    autofocus: autofocus === true ? "end" : autofocus,
    editorProps: {
      attributes: {
        class: "kn-content kn-editor-surface",
        role: "textbox",
        "aria-multiline": "true",
        "aria-label": ariaLabel ?? (isFull ? "Page content" : "Comment"),
      },
    },
    onUpdate: ({ editor: e }) => {
      onChangeRef.current?.(snapshot(e));
    },
  });

  useEffect(() => {
    editorRef.current = editor;
  }, [editor]);

  useEffect(() => {
    if (editor && editor.isEditable !== editable) editor.setEditable(editable, false);
  }, [editor, editable]);

  useImperativeHandle(
    ref,
    () => ({
      get editor() {
        return editor;
      },
      getValue: () => (editor ? snapshot(editor) : null),
      setContent: (next) => {
        editor?.commands.setContent(htmlToInitialContent(next), false);
      },
      clear: () => {
        editor?.commands.clearContent(false);
      },
      focus: (position) => {
        editor?.commands.focus(position);
      },
      isEmpty: () => (editor ? isEmptyDoc(editor.getJSON() as PMDoc) : true),
    }),
    [editor]
  );

  const counts = useEditorState({
    editor,
    selector: ({ editor: e }) =>
      e && isFull
        ? {
            words: e.storage.characterCount?.words?.() ?? 0,
            characters: e.storage.characterCount?.characters?.() ?? 0,
          }
        : { words: 0, characters: 0 },
  });

  function onFilesChosen(files: FileList | null) {
    if (!editor || !files || files.length === 0) return;
    uploadImageFiles(editor, Array.from(files), {
      getUpload: () => uploadRef.current,
      onError: (m) => toastRef.current.error(m),
    });
  }

  const words = counts?.words ?? 0;
  const canUpload = Boolean(uploadImage);

  return (
    <div
      className={cn(
        "kn-editor",
        isFull ? "kn-editor-full" : "kn-editor-compact",
        !editable && "is-readonly",
        className
      )}
    >
      {editor && isFull && editable && (
        <Toolbar
          editor={editor}
          onOpenLink={openLink}
          onPickImage={pickImage}
          canUpload={canUpload}
        />
      )}
      <EditorContent editor={editor} className="kn-editor-body" />
      {editor && editable && (
        <>
          <TextBubble
            editor={editor}
            onOpenLink={openLink}
            onRequestComment={onRequestComment}
            allowComment={isFull}
          />
          {isFull && (
            <>
              <TableBubble editor={editor} />
              <StatusBubble editor={editor} />
              <PanelBubble editor={editor} />
              <CodeBlockBubble editor={editor} />
              <ImageBubble editor={editor} />
            </>
          )}
        </>
      )}
      {linkUi && editor && (
        <LinkPopover editor={editor} state={linkUi} onClose={() => setLinkUi(null)} />
      )}
      {canUpload && (
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          hidden
          onChange={(e) => {
            onFilesChosen(e.target.files);
            e.target.value = "";
          }}
        />
      )}
      {isFull && editable && editor && (
        <div className="kn-editor-footer">
          {words} {words === 1 ? "word" : "words"}
        </div>
      )}
    </div>
  );
});
