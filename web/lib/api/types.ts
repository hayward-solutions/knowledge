export type Role = "admin" | "editor" | "viewer";
export type Visibility = "private" | "internal";
export type PageStatus = "draft" | "published" | "trashed";
export type WatchTargetType = "page" | "space";
export type PMDoc = Record<string, unknown>;

export interface User {
  id: string;
  email: string;
  display_name: string;
  is_system_admin: boolean;
  disabled_at?: string | null;
  created_at: string;
  updated_at?: string;
}

export interface UserRef {
  id: string;
  display_name: string;
  email?: string;
}

export interface Space {
  id: string;
  key: string;
  name: string;
  description: string;
  icon: string | null;
  visibility: Visibility;
  home_page_id: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
  archived_at: string | null;
  deleted_at?: string | null;
  role?: Role | null;
  member_count?: number;
  page_count?: number;
}

export interface SpaceMember {
  space_id?: string;
  user_id: string;
  user: UserRef;
  role: Role;
  created_at: string;
}

export interface PageSummary {
  id: string;
  title: string;
  slug: string;
  parent_id: string | null;
  position: number;
  status: PageStatus;
  has_children: boolean;
}

export interface Page {
  id: string;
  space_id: string;
  space_key?: string;
  parent_id: string | null;
  title: string;
  slug: string;
  position: number;
  status: PageStatus;
  version: number;
  content_json: PMDoc | null;
  content_html: string;
  content_text?: string;
  created_by: string;
  updated_by: string | null;
  created_by_user?: UserRef | null;
  updated_by_user?: UserRef | null;
  created_at: string;
  updated_at: string;
  published_at: string | null;
  trashed_at: string | null;
  trashed_by: string | null;
}

export interface MyDraftRef {
  updated_at: string;
  base_version: number;
}

export interface PageDetail extends Page {
  labels: Label[];
  breadcrumbs: PageSummary[];
  children: PageSummary[];
  my_draft: MyDraftRef | null;
  editors: UserRef[];
  starred: boolean;
  watching: boolean;
  my_role?: Role | null;
}

export interface TrashedPage extends PageSummary {
  trashed_at: string;
  trashed_by: string | null;
  trashed_by_user?: UserRef | null;
}

export interface PageVersion {
  id: string;
  page_id: string;
  version: number;
  title: string;
  content_json: PMDoc | null;
  content_html: string;
  author_id: string;
  author?: UserRef | null;
  message: string | null;
  created_at: string;
}

export type PageVersionSummary = Omit<PageVersion, "content_json" | "content_html"> & {
  content_json?: PMDoc | null;
  content_html?: string;
};

export interface PageDraft {
  page_id: string;
  user_id: string;
  title: string;
  content_json: PMDoc | null;
  content_html: string;
  base_version: number;
  updated_at: string;
}

export interface Label {
  id: string;
  name: string;
  created_at?: string;
  page_count?: number;
}

export interface InlineAnchor {
  quoted_text: string;
  prefix?: string;
  suffix?: string;
}

export interface Comment {
  id: string;
  page_id: string;
  parent_id: string | null;
  author_id: string;
  author?: UserRef | null;
  body_json: PMDoc | null;
  body_html: string;
  inline_anchor: InlineAnchor | null;
  resolved_at: string | null;
  resolved_by: string | null;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
  replies: Comment[];
}

export interface Attachment {
  id: string;
  page_id: string;
  space_id: string;
  uploader_id: string;
  uploader?: UserRef | null;
  filename: string;
  content_type: string;
  size: number;
  created_at: string;
  deleted_at?: string | null;
}

export interface Template {
  id: string;
  space_id: string | null;
  space_key?: string | null;
  name: string;
  description: string;
  content_json: PMDoc | null;
  content_html: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface Notification {
  id: string;
  user_id: string;
  type: string;
  actor_id: string;
  actor?: UserRef | null;
  space_id: string | null;
  space_key?: string | null;
  page_id: string | null;
  page_title?: string | null;
  comment_id: string | null;
  payload: Record<string, unknown>;
  read_at: string | null;
  created_at: string;
}

export interface Activity {
  id: string;
  space_id: string;
  page_id: string | null;
  page_title?: string | null;
  actor_id: string;
  actor?: UserRef | null;
  type: string;
  payload: Record<string, unknown>;
  created_at: string;
}

export interface SearchHit {
  page: PageSummary;
  space_key: string;
  snippet: string;
  updated_at: string;
  author: UserRef | null;
}

export interface SearchResult {
  hits: SearchHit[];
  total: number;
  page: number;
  limit: number;
}

export interface QuickSearchHit {
  page: PageSummary;
  space_key: string;
  space_name?: string;
}

export interface DashboardPage extends PageSummary {
  space_key: string;
  space_name?: string;
  updated_at: string;
  author?: UserRef | null;
}

export interface Dashboard {
  recent_updates: DashboardPage[];
  recently_viewed: DashboardPage[];
  my_drafts: DashboardPage[];
  starred: DashboardPage[];
}

export interface VersionConflict {
  error: string;
  code: "version_conflict";
  current_version: number;
}

export interface OIDCConfig {
  enabled: boolean;
  provider_name?: string;
}

export interface EditingState {
  editors: UserRef[];
}

export interface WatchState {
  watching: boolean;
}

export interface AdminSpace extends Space {
  member_count: number;
  page_count: number;
}

export interface DraftInput {
  title: string;
  content_json: PMDoc | null;
  content_html: string;
  base_version: number;
}

export interface PublishInput extends DraftInput {
  message?: string;
  force?: boolean;
}

export interface MovePageInput {
  parent_id: string | null;
  after_id?: string | null;
  position?: number;
}

export interface CreatePageInput {
  title: string;
  parent_id?: string | null;
  template_id?: string | null;
}

export interface CreateSpaceInput {
  key: string;
  name: string;
  description?: string;
  icon?: string | null;
  visibility?: Visibility;
}

export interface UpdateSpaceInput {
  name?: string;
  description?: string;
  icon?: string | null;
  visibility?: Visibility;
  home_page_id?: string | null;
}

export interface TemplateInput {
  name: string;
  description: string;
  content_json: PMDoc | null;
  content_html: string;
}

export interface CommentInput {
  body_json: PMDoc | null;
  body_html: string;
  parent_id?: string | null;
  inline_anchor?: InlineAnchor | null;
}

export interface SearchParams {
  q: string;
  space?: string;
  label?: string;
  author?: string;
  page?: number;
  limit?: number;
}

export interface ImportResult {
  pages: PageSummary[];
}

export type ExportFormat = "md" | "html";
