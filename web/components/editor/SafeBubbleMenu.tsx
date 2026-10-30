"use client";

import { BubbleMenuPlugin, type BubbleMenuPluginProps } from "@tiptap/extension-bubble-menu";
import type { Editor } from "@tiptap/core";
import { useEffect, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

type Optional<T, K extends keyof T> = Pick<Partial<T>, K> & Omit<T, K>;

export type SafeBubbleMenuProps = Omit<
  Optional<BubbleMenuPluginProps, "pluginKey">,
  "element" | "editor"
> & {
  editor: Editor | null;
  className?: string;
  children: ReactNode;
};

export function SafeBubbleMenu({
  editor,
  className,
  children,
  pluginKey = "bubbleMenu",
  tippyOptions,
  updateDelay,
  shouldShow = null,
}: SafeBubbleMenuProps) {
  const [element] = useState<HTMLDivElement | null>(() =>
    typeof document === "undefined" ? null : document.createElement("div")
  );

  useEffect(() => {
    if (!element) return;
    if (className) element.className = className;
  }, [element, className]);

  useEffect(() => {
    if (!element || !editor || editor.isDestroyed) return;
    element.style.visibility = "hidden";
    const plugin = BubbleMenuPlugin({
      updateDelay,
      editor,
      element,
      pluginKey,
      shouldShow,
      tippyOptions,
    });
    editor.registerPlugin(plugin);
    return () => {
      if (!editor.isDestroyed) editor.unregisterPlugin(pluginKey);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor, element]);

  if (!element) return null;
  return createPortal(children, element);
}
