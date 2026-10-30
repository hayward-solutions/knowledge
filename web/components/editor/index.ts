export { Editor, type EditorProps } from "./Editor";
export { ContentView, useHeadings, type ContentViewProps } from "./ContentView";
export { CommentMark } from "./extensions/comment-mark";
export { createExtensions } from "./extensions";
export { anchorCommentInContent, removeCommentFromContent } from "./commentAnchor";
export { CONTRACT_SAMPLES } from "./__contract__";
export {
  pageLinkHref,
  type CommentSelection,
  type EditorChange,
  type EditorHandle,
  type EditorValue,
  type PageLinkItem,
  type SearchPagesFn,
  type SearchUsersFn,
  type UploadImageFn,
  type UploadedImage,
} from "./types";
export type { HeadingInfo } from "@/lib/content/headings";
