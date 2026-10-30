import { ReactRenderer } from "@tiptap/react";
import type { SuggestionKeyDownProps, SuggestionProps } from "@tiptap/suggestion";
import tippy, { type Instance } from "tippy.js";
import {
  SuggestionList,
  type SuggestionListHandle,
  type SuggestionListItem,
  type SuggestionListProps,
} from "./SuggestionList";

export interface SuggestionRenderOptions<T> {
  toItem: (item: T) => SuggestionListItem<T>;
  emptyLabel?: (query: string) => string;
  hideWhenEmpty?: boolean;
}

export function createSuggestionRender<T>(options: SuggestionRenderOptions<T>) {
  return () => {
    let component: ReactRenderer<SuggestionListHandle, SuggestionListProps<T>> | null = null;
    let popup: Instance | null = null;
    let dismissed = false;

    const toProps = (props: SuggestionProps<T, T>): SuggestionListProps<T> => ({
      items: props.items.map(options.toItem),
      command: (item) => props.command(item.data),
      emptyLabel: options.emptyLabel?.(props.query),
    });

    const sync = (props: SuggestionProps<T, T>) => {
      if (!popup) return;
      if (props.clientRect) {
        popup.setProps({ getReferenceClientRect: props.clientRect as () => DOMRect });
      }
      if (dismissed || (options.hideWhenEmpty && props.items.length === 0)) popup.hide();
      else popup.show();
    };

    return {
      onStart(props: SuggestionProps<T, T>) {
        dismissed = false;
        component = new ReactRenderer(
          SuggestionList as unknown as React.ComponentType<SuggestionListProps<T>>,
          { props: toProps(props), editor: props.editor }
        ) as unknown as ReactRenderer<SuggestionListHandle, SuggestionListProps<T>>;
        if (!props.clientRect) return;
        popup = tippy(document.body, {
          getReferenceClientRect: props.clientRect as () => DOMRect,
          appendTo: () => document.body,
          content: component.element,
          showOnCreate: true,
          interactive: true,
          trigger: "manual",
          placement: "bottom-start",
          theme: "knowledge",
          arrow: false,
          offset: [0, 6],
          maxWidth: "none",
          animation: false,
        });
        sync(props);
      },
      onUpdate(props: SuggestionProps<T, T>) {
        dismissed = false;
        component?.updateProps(toProps(props));
        sync(props);
      },
      onKeyDown(props: SuggestionKeyDownProps) {
        if (props.event.key === "Escape") {
          dismissed = true;
          popup?.hide();
          return true;
        }
        if (dismissed) return false;
        return component?.ref?.onKeyDown(props.event) ?? false;
      },
      onExit() {
        popup?.destroy();
        component?.destroy();
        popup = null;
        component = null;
      },
    };
  };
}

export function debouncedAsync<A, R>(
  fn: (arg: A) => Promise<R[]>,
  ms: number
): (arg: A) => Promise<R[]> {
  let seq = 0;
  let last: R[] = [];
  return (arg: A) => {
    const mine = ++seq;
    return new Promise<R[]>((resolve) => {
      setTimeout(async () => {
        if (mine !== seq) {
          resolve(last);
          return;
        }
        try {
          const result = await fn(arg);
          if (mine === seq) last = result;
          resolve(mine === seq ? result : last);
        } catch {
          resolve([]);
        }
      }, ms);
    });
  };
}
