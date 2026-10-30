"use client";

import { useEffect, useRef } from "react";
import { apiUrl, notifyUnauthenticated } from "@/lib/api/client";

type Handlers = Record<string, (data: any) => void>;

const MIN_DELAY_MS = 1000;
const MAX_DELAY_MS = 30000;
const MAX_CONSECUTIVE_FAILURES = 6;
const STABLE_AFTER_MS = 5000;

export function useEventStream(path: string | null, handlers: Handlers) {
  const ref = useRef<Handlers>(handlers);
  ref.current = handlers;
  const names = Object.keys(handlers).sort().join(",");

  useEffect(() => {
    if (!path) return;
    let es: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let probe: AbortController | null = null;
    let delay = MIN_DELAY_MS;
    let failures = 0;
    let stopped = false;
    const eventNames = names ? names.split(",") : [];

    const dispatch = (name: string) => (e: MessageEvent) => {
      const fn = ref.current[name];
      if (!fn) return;
      let data: unknown = e.data;
      try {
        data = JSON.parse(e.data);
      } catch {
        data = e.data;
      }
      fn(data);
    };

    const retry = () => {
      if (stopped) return;
      failures += 1;
      if (failures >= MAX_CONSECUTIVE_FAILURES) {
        stopped = true;
        return;
      }
      timer = setTimeout(connect, delay);
      delay = Math.min(delay * 2, MAX_DELAY_MS);
    };

    const open = () => {
      if (stopped) return;
      const source = new EventSource(apiUrl(path), { withCredentials: true });
      es = source;
      let openedAt = 0;
      source.onopen = () => {
        openedAt = Date.now();
        delay = MIN_DELAY_MS;
      };
      for (const name of eventNames) {
        source.addEventListener(name, dispatch(name) as EventListener);
      }
      source.onerror = () => {
        source.close();
        if (es === source) es = null;
        if (stopped) return;
        if (openedAt && Date.now() - openedAt >= STABLE_AFTER_MS) {
          failures = 0;
          delay = MIN_DELAY_MS;
        }
        retry();
      };
    };

    const connect = () => {
      if (stopped) return;
      const ctrl = new AbortController();
      probe = ctrl;
      fetch(apiUrl(path), {
        credentials: "include",
        headers: { Accept: "text/event-stream" },
        signal: ctrl.signal,
      })
        .then((res) => {
          ctrl.abort();
          if (stopped) return;
          if (res.status === 401 || res.status === 403) {
            stopped = true;
            if (res.status === 401) notifyUnauthenticated(path);
            return;
          }
          if (res.status === 200) {
            open();
            return;
          }
          retry();
        })
        .catch(() => {
          if (stopped || ctrl.signal.aborted) return;
          retry();
        });
    };

    connect();

    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
      probe?.abort();
      es?.close();
    };
  }, [path, names]);
}
