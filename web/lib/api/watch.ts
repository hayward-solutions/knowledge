import { api } from "./client";
import type { WatchState, WatchTargetType } from "./types";

export function getWatchState(
  type: WatchTargetType,
  id: string
): Promise<WatchState> {
  return api<WatchState>(`/watch/${type}/${id}`);
}

export function watchTarget(type: WatchTargetType, id: string): Promise<void> {
  return api<void>(`/watch/${type}/${id}`, { method: "PUT" });
}

export function unwatchTarget(type: WatchTargetType, id: string): Promise<void> {
  return api<void>(`/watch/${type}/${id}`, { method: "DELETE" });
}
