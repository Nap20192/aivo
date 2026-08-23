import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Served by the Go binary at /admin.
export default defineConfig({
  base: "/admin/",
  plugins: [react()],
  server: {
    fs: { allow: [".."] },
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
