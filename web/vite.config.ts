import { writeFileSync } from 'node:fs';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [
    react(),
    {
      // web.go embeds dist/, which must exist even before the first build
      // (for `go vet` and the Go tests). A committed dist/.gitkeep guarantees
      // that; put it back after Vite empties the folder.
      name: 'keep-dist-placeholder',
      closeBundle() {
        writeFileSync('dist/.gitkeep', '');
      },
    },
  ],
  build: { outDir: 'dist', emptyOutDir: true },
  // `npm run dev` serves the UI with hot reload and forwards /api to the Go
  // server (e.g. the local Docker stack on :8080).
  server: { proxy: { '/api': 'http://localhost:8080' } },
});
