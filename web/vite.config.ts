import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],

  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },

  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },

  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://36.152.34.250:18083",
        changeOrigin: true,
      },
    },
  },
})
