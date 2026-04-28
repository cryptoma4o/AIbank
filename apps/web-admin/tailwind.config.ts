import type { Config } from "tailwindcss";

// Банковская монохромная палитра для оператора: акцент primary #1E40AF,
// нейтральные серые, поверхность светло-серая.  По сравнению с
// web-onboarding (синий 2563EB) — приглушённее и серьёзнее: оператор
// проводит за этим экраном весь рабочий день.
const config: Config = {
  content: ["./src/**/*.{js,ts,jsx,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: "#1E40AF",
          hover: "#1D3A95",
          50: "#EFF4FB",
          100: "#DBE6F6",
          700: "#1E40AF",
        },
        surface: "#F4F5F7",
        sidebar: "#0F172A",
        success: "#15803D",
        warning: "#B45309",
        danger: "#B91C1C",
      },
      fontSize: {
        // Чуть меньше дефолта — operator UI требует больше плотности.
        xs: ["0.75rem", { lineHeight: "1rem" }],
        sm: ["0.8125rem", { lineHeight: "1.125rem" }],
        base: ["0.875rem", { lineHeight: "1.25rem" }],
      },
    },
  },
  plugins: [],
};
export default config;
