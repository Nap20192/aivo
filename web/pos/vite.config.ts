import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Served at /pos by the Go server.
export default defineConfig({
  base: "/pos/",
  plugins: [react()],
  server: {
    // design tokens live one level up in web/design-system
    fs: { allow: [".."] },
    proxy: { "/api": "http://localhost:8080" },
  },
});
