/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./*.html",
    "./*.js",
    "./modules/**/*.js",
    "./ui/**/*.js",
    "./services/**/*.js",
    "./state/**/*.js",
    "./utils/**/*.js",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: "#0df259",
          hover: "#0bd34d",
          light: "rgba(13, 242, 89, 0.15)",
        },
        background: {
          dark: "#0c1810",
        },
        surface: {
          dark: "rgba(16, 35, 22, 0.6)",
          light: "rgba(34, 73, 47, 0.5)",
          hover: "rgba(46, 87, 58, 0.9)",
        },
      },
      fontFamily: {
        sans: ["Inter", "sans-serif"],
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "Consolas",
          "monospace",
        ],
      },
      borderRadius: {
        xl: "1rem",
        "2xl": "1.5rem",
      },
      boxShadow: {
        glow: "0 0 20px rgba(13, 242, 89, 0.2)",
        "glow-lg": "0 0 30px rgba(13, 242, 89, 0.3)",
      },
    },
  },
  plugins: [
    require("@tailwindcss/forms"),
    require("@tailwindcss/container-queries"),
  ],
};
