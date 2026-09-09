import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig, type Plugin } from 'vite';

/**
 * Keeps dist/.gitkeep in place after a build.
 *
 * main.go embeds the built interface with `//go:embed all:frontend/dist`, and
 * that pattern fails to compile when the directory does not exist. Since dist
 * is build output and therefore gitignored, a fresh clone has no such
 * directory and `go build` and `go test ./...` both fail before they start —
 * on a checkout where nothing is actually wrong.
 *
 * Committing a placeholder solves that, but Vite empties the output directory
 * on every build and takes the placeholder with it, leaving a deleted file in
 * every working tree. Writing it back afterwards keeps both properties: the
 * directory always exists, and git stays clean.
 */
function keepDistDirectory(): Plugin {
  return {
    name: 'playerone-keep-dist',
    apply: 'build',
    closeBundle() {
      const dist = resolve(__dirname, 'dist');
      mkdirSync(dist, { recursive: true });
      writeFileSync(
        resolve(dist, '.gitkeep'),
        '# Keeps this directory present for //go:embed in main.go.\n' +
          '# Everything else here is build output and is gitignored.\n',
      );
    },
  };
}

export default defineConfig({
  plugins: [keepDistDirectory()],
});
