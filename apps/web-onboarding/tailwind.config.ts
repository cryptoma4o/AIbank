import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{js,ts,jsx,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        primary: { DEFAULT: "#1B4FCC", hover: "#1640A8" },
        surface: "#F8FAFC",
      },
    },
  },
  plugins: [],
};
export default config;
