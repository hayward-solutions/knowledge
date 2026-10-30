import { quickSearch } from "@/lib/api/search";
import { searchUsers } from "@/lib/api/users";
import type { SearchPagesFn, SearchUsersFn } from "./types";

export const defaultSearchUsers: SearchUsersFn = (q) => searchUsers(q);

export const defaultSearchPages: SearchPagesFn = async (q) => {
  const hits = await quickSearch(q);
  return hits.map((hit) => ({
    id: hit.page.id,
    title: hit.page.title,
    slug: hit.page.slug,
    spaceKey: hit.space_key,
    spaceName: hit.space_name,
  }));
};
