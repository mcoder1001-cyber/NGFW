import { defineConfig } from 'drizzle-kit';

// `pnpm db:generate` only diffs src/db/schema.ts against migrations/ — it never connects to a database.
export default defineConfig({
  dialect: 'postgresql',
  schema: './src/db/schema.ts',
  out: './migrations',
});
