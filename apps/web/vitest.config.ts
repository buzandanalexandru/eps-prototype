import { defineConfig } from "vitest/config";
import path from "node:path";

export default defineConfig({
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  test: {
    environment: "node",
    include: ["tests/unit/**/*.test.ts", "tests/unit/**/*.test.tsx"],
    restoreMocks: true,
    // Unit tests never call OpenStreetMap (tests inject a fake tile fetcher).
    env: { EPS_MAP_TILES: "off" },
  },
});
