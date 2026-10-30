import type { Extensions } from "@tiptap/core";
import CharacterCount from "@tiptap/extension-character-count";
import type { MentionOptions } from "@tiptap/extension-mention";
import CodeBlockLowlight from "@tiptap/extension-code-block-lowlight";
import Highlight from "@tiptap/extension-highlight";
import Link from "@tiptap/extension-link";
import Placeholder from "@tiptap/extension-placeholder";
import { Table } from "@tiptap/extension-table";
import { TableCell } from "@tiptap/extension-table-cell";
import { TableHeader } from "@tiptap/extension-table-header";
import { TableRow } from "@tiptap/extension-table-row";
import TaskList from "@tiptap/extension-task-list";
import TextAlign from "@tiptap/extension-text-align";
import Typography from "@tiptap/extension-typography";
import Underline from "@tiptap/extension-underline";
import StarterKit from "@tiptap/starter-kit";
import { lowlight } from "../lowlight";
import type { SearchPagesFn, SearchUsersFn } from "../types";
import type { UploadHooks } from "./upload";
import { CommentMark } from "./comment-mark";
import { Expand, ExpandBody, ExpandSummary } from "./expand";
import { HeadingWithId } from "./heading";
import { EditorImage } from "./image";
import { LinkShortcut } from "./link-shortcut";
import { KnMention, mentionSuggestion } from "./mention";
import { PageLink } from "./page-link";
import { Panel } from "./panel";
import { SlashCommand } from "./slash-command";
import { Status } from "./status";
import { KnTaskItem } from "./task";
import { Toc } from "./toc";
import { ImageUpload } from "./upload";

export interface ExtensionConfig {
  variant: "full" | "compact";
  placeholder: string;
  searchUsers: SearchUsersFn;
  searchPages: SearchPagesFn;
  upload: UploadHooks;
  openLink: () => void;
  pickImage: () => void;
}

export function createExtensions(config: ExtensionConfig): Extensions {
  const extensions: Extensions = [
    StarterKit.configure({
      heading: false,
      codeBlock: false,
    }),
    HeadingWithId.configure({ levels: [1, 2, 3, 4, 5, 6] }),
    CodeBlockLowlight.configure({
      lowlight,
      defaultLanguage: null,
      languageClassPrefix: "language-",
    }),
    Underline,
    Highlight.configure({ multicolor: false }),
    TextAlign.configure({ types: ["heading", "paragraph"] }),
    Typography,
    Link.configure({
      openOnClick: false,
      autolink: true,
      linkOnPaste: true,
      defaultProtocol: "https",
      protocols: ["mailto"],
      HTMLAttributes: { target: "_blank", rel: "noopener noreferrer", class: null },
    }),
    Placeholder.configure({
      placeholder: ({ node }) => {
        if (node.type.name === "heading") return "Heading";
        if (node.type.name === "expandSummary") return "Expand title";
        return config.placeholder;
      },
    }),
    CharacterCount,
    Table.configure({ resizable: true }),
    TableRow,
    TableHeader,
    TableCell,
    TaskList,
    KnTaskItem.configure({ nested: true }),
    EditorImage.configure({ inline: false, allowBase64: false }),
    KnMention.configure({
      suggestion: mentionSuggestion(config.searchUsers) as unknown as MentionOptions["suggestion"],
    }),
    PageLink.configure({ search: config.searchPages }),
    Panel,
    Expand,
    ExpandSummary,
    ExpandBody,
    Status,
    Toc,
    CommentMark,
    ImageUpload.configure(config.upload),
    LinkShortcut.configure({ open: config.openLink }),
  ];
  if (config.variant === "full") {
    extensions.push(
      SlashCommand.configure({
        hasUpload: () => Boolean(config.upload.getUpload()),
        pickImage: config.pickImage,
      })
    );
  }
  return extensions;
}

export { CommentMark } from "./comment-mark";
export { PANEL_COLORS, type PanelColor } from "./panel";
export { STATUS_COLORS, type StatusColor } from "./status";
export { uploadImageFiles } from "./upload";
