import type { Config } from "tailwindcss";

// Банковский UI-токен-набор: спокойный синий primary, нейтральные серые,
// мягкая поверхность.  Цвета подобраны под product-vision.md § 3
// (доверие, читаемость, минимум цвета).
const config: Config = {
  content: ["./src/**/*.{js,ts,jsx,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: "#2563EB",
          hover: "#1D4ED8",
          50: "#EFF6FF",
          100: "#DBEAFE",
        },
        surface: "#F8FAFC",
        success: "#16A34A",
        warning: "#D97706",
        danger: "#DC2626",
      },
    },
  },
  plugins: [],
};
export default config;
