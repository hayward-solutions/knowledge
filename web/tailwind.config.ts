import type { Config } from "tailwindcss";
import typography from "@tailwindcss/typography";

export default {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      fontFamily: {
        sans: [
          "Inter",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "Helvetica Neue",
          "Arial",
          "sans-serif",
        ],
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "Consolas",
          "monospace",
        ],
      },
      colors: {
        brand: {
          50: "var(--brand-50)",
          100: "var(--brand-100)",
          200: "#85B8FF",
          300: "#579DFF",
          400: "#388BFF",
          500: "#1D7AFC",
          600: "#0C66E4",
          700: "var(--brand-700)",
          800: "var(--brand-800)",
          900: "#1C2B41",
        },
        ink: {
          0: "var(--ink-0)",
          50: "var(--ink-50)",
          100: "var(--ink-100)",
          200: "var(--ink-200)",
          300: "var(--ink-300)",
          400: "var(--ink-400)",
          500: "var(--ink-500)",
          600: "var(--ink-600)",
          700: "var(--ink-700)",
          800: "var(--ink-800)",
          900: "var(--ink-900)",
        },
        danger: {
          50: "var(--danger-50)",
          100: "var(--danger-100)",
          200: "#FFBDB8",
          500: "#E2483D",
          600: "var(--danger-600)",
          700: "var(--danger-700)",
        },
        warning: {
          50: "var(--warning-50)",
          100: "var(--warning-100)",
          500: "#E2B203",
          600: "var(--warning-600)",
          700: "var(--warning-700)",
        },
        success: {
          50: "var(--success-50)",
          100: "var(--success-100)",
          500: "#22A06B",
          600: "var(--success-600)",
          700: "var(--success-700)",
        },
        purple: {
          50: "var(--purple-50)",
          100: "var(--purple-100)",
          500: "#8270DB",
          600: "var(--purple-600)",
          700: "var(--purple-700)",
        },
        teal: {
          50: "var(--teal-50)",
          500: "#1D7F8C",
          600: "var(--teal-600)",
        },
        magenta: {
          50: "var(--magenta-50)",
          500: "#CD519D",
          600: "var(--magenta-600)",
        },
      },
      boxShadow: {
        card: "0 1px 2px rgba(9,30,66,0.08)",
        raised:
          "0 1px 1px rgba(9,30,66,0.25), 0 0 1px rgba(9,30,66,0.31)",
        overlay:
          "0 8px 12px rgba(9,30,66,0.15), 0 0 1px rgba(9,30,66,0.31)",
        drawer: "-8px 0 24px rgba(9,30,66,0.12)",
        focus: "0 0 0 2px #4C9AFF",
      },
      borderRadius: {
        xs: "3px",
      },
      keyframes: {
        "fade-in": {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        "slide-in-right": {
          "0%": { transform: "translateX(16px)", opacity: "0" },
          "100%": { transform: "translateX(0)", opacity: "1" },
        },
        "scale-in": {
          "0%": { transform: "scale(0.96)", opacity: "0" },
          "100%": { transform: "scale(1)", opacity: "1" },
        },
      },
      animation: {
        "fade-in": "fade-in 150ms ease-out",
        "slide-in-right": "slide-in-right 220ms cubic-bezier(0.2, 0, 0, 1)",
        "scale-in": "scale-in 160ms cubic-bezier(0.2, 0, 0, 1)",
      },
    },
  },
  plugins: [typography],
} satisfies Config;
