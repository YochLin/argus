import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Builds straight into internal/web/dist rather than a local dist/ here —
// go:embed patterns can't reach outside their package directory with "..",
// so the Go side that embeds this build (internal/web/server.go) needs the
// output to already live inside internal/web/. This project (source) stays
// at the repo root per docs/phase-5-web-dashboard.md's "repo 新增 web/
// 目錄放前端專案"; only the build artifact's location is unusual.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      // `npm run dev` proxies API calls to a locally running
      // `WEB_ADDR=127.0.0.1:8090 go run ./cmd/bot` so the SPA can be
      // developed with hot reload against real data.
      "/api": "http://127.0.0.1:8090",
    },
  },
});
