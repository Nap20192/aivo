import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Served at /pos by the Go server.
export default defineConfig({
  base: "/pos/",
  // build id versions the service-worker cache so old bundles get dropped on deploy
  define: { __BUILD_ID__: JSON.stringify(Date.now().toString(36)) },
  plugins: [react()],
  server: {
    // design tokens live one level up in web/design-system
    fs: { allow: [".."] },
    proxy: { "/api": "http://localhost:8080" },
  },
});
