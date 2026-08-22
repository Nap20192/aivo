import { defineConfig } from "vitest/config";

// Also runs the shared design-system module tests (plain TS, no build of
// their own) from this app's toolchain.
export default defineConfig({
  test: {
    include: ["src/**/*.test.ts", "../design-system/shared/*.test.ts"],
  },
});
