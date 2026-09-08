import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // Без минификации бандл крупнее на пару сотен килобайт, зато в стеке
    // аварийного экрана видны настоящие имена функций и компонентов.
    minify: false,
  },
});
