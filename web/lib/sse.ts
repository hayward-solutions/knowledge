"use client";

import { useEffect, useRef } from "react";
import { apiUrl } from "@/lib/api/client";

type Handlers = Record<string, (data: any) => void>;

const MIN_DELAY_MS = 1000;
const MAX_DELAY_MS = 30000;

export function useEventStream(path: string | null, handlers: Handlers) {
  const ref = useRef<Handlers>(handlers);
  ref.current = handlers;
  const names = Object.keys(handlers).sort().join(",");

  useEffect(() => {
    if (!path) return;
    let es: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let delay = MIN_DELAY_MS;
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

    const connect = () => {
      if (stopped) return;
      es = new EventSource(apiUrl(path), { withCredentials: true });
      es.onopen = () => {
        delay = MIN_DELAY_MS;
      };
      for (const name of eventNames) {
        es.addEventListener(name, dispatch(name) as EventListener);
      }
      es.onerror = () => {
        es?.close();
        es = null;
        if (stopped) return;
        timer = setTimeout(connect, delay);
        delay = Math.min(delay * 2, MAX_DELAY_MS);
      };
    };

    connect();

    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
      es?.close();
    };
  }, [path, names]);
}
