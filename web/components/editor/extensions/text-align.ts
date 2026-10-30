import TextAlign from "@tiptap/extension-text-align";

const ALIGNMENTS = ["left", "center", "right", "justify"];

export const KnTextAlign = TextAlign.extend({
  addGlobalAttributes() {
    const { types, defaultAlignment } = this.options;
    return [
      {
        types,
        attributes: {
          textAlign: {
            default: defaultAlignment,
            parseHTML: (element: HTMLElement) => {
              const value = element.getAttribute("data-align") || element.style.textAlign;
              return value && ALIGNMENTS.includes(value) ? value : defaultAlignment;
            },
            renderHTML: (attributes: Record<string, unknown>) =>
              attributes.textAlign && attributes.textAlign !== defaultAlignment
                ? { "data-align": attributes.textAlign as string }
                : {},
          },
        },
      },
    ];
  },
});
