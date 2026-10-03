// Use TypeScript's decorator metadata in development, just as in production.
// tsx/esbuild does not emit the metadata used by Nest's constructor injection.
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const cwd = fileURLToPath(new URL('../', import.meta.url));
const tsc = require.resolve('typescript/bin/tsc');
const children = new Set();
let stopping = false;

function stop(code) {
  if (stopping) return;
  stopping = true;
  process.exitCode = code;
  for (const child of children) child.kill('SIGTERM');
}

function run(args) {
  const child = spawn(process.execPath, args, { cwd, stdio: 'inherit' });
  children.add(child);
  child.on('error', (error) => {
    console.error(error);
    stop(1);
  });
  child.on('exit', (code, signal) => {
    children.delete(child);
    if (!stopping) stop(code ?? (signal ? 1 : 0));
  });
  return child;
}

process.on('SIGINT', () => stop(130));
process.on('SIGTERM', () => stop(143));

// Complete the first compilation before loading the API: dist may not exist,
// or may contain an older build. Do not start when compilation fails.
const initial = run([tsc, '-p', 'tsconfig.build.json', '--noEmitOnError']);
initial.removeAllListeners('exit');
initial.on('exit', (code) => {
  children.delete(initial);
  if (stopping) return;
  if (code !== 0) return stop(code ?? 1);
  run([tsc, '-p', 'tsconfig.build.json', '--watch', '--preserveWatchOutput', '--noEmitOnError']);
  run(['--watch', 'dist/main.js']);
});
