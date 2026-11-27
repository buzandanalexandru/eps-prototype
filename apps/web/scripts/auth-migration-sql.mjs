// Prints the SQL Better Auth needs for our plugin set, by asking Better Auth itself
// (getMigrations) against an empty database whose search_path is the control schema.
// Usage: AUTH_GEN_DATABASE_URL=postgres://... node scripts/auth-migration-sql.mjs > out.sql
// Its output was pasted into db/migrations/00002_auth.sql (design D0.2).
import pg from "pg";
import { getMigrations } from "better-auth/db/migration";
import { jwt, magicLink } from "better-auth/plugins";

const pool = new pg.Pool({
  connectionString: process.env.AUTH_GEN_DATABASE_URL,
  options: "-c search_path=control",
});
await pool.query("create schema if not exists control");
const { compileMigrations } = await getMigrations({
  database: pool,
  secret: "x".repeat(32),
  rateLimit: { enabled: true, storage: "database" },
  plugins: [magicLink({ sendMagicLink: async () => {} }), jwt()],
});
process.stdout.write(await compileMigrations());
await pool.end();
