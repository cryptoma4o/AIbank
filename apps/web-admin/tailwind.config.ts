import type { Config } from "tailwindcss";
const config: Config = {
  content: ["./src/**/*.{js,ts,jsx,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        primary: { DEFAULT: "#1B4FCC", hover: "#1640A8" },
        sidebar: "#0F172A",
      },
    },
  },
  plugins: [],
};
export default config;
