import { defineConfig } from 'rolldown';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = join(root, 'dist');

export default defineConfig({
  input: {
    main: join(root, 'js', 'main.js'),
    'update-package-worker': join(root, 'js', 'workers', 'update-package-worker.js'),
  },
  output: {
    dir: outDir,
    format: 'esm',
    codeSplitting: true,
    entryFileNames: 'js/[name].js',
    assetFileNames: 'assets/[name]-[hash][extname]',
    minify: true,
  },
  platform: 'browser',
  treeshake: true,
});
