// npm run seed: applies the sample organisers to SEED_DATABASE_URL (else DATABASE_URL). CI runs it
// after migrations on previews and production, as the branch owner; every write still runs under
// the eps_web role through withTx. Prints counts only, never the connection string.
import pg from "pg";
import { demoEmailDomain } from "@/lib/env";
import { applySeed } from "./apply";

async function main() {
  const url = process.env.SEED_DATABASE_URL || process.env.DATABASE_URL;
  if (!url) {
    console.error("seed: SEED_DATABASE_URL or DATABASE_URL is required");
    process.exit(2);
  }
  const pool = new pg.Pool({ connectionString: url, max: 1 });
  const started = Date.now();
  try {
    const results = await applySeed(pool, { demoDomain: demoEmailDomain() });
    for (const r of results) {
      console.log(`seed: sample organiser ${r.organiser} (${r.id}): ${r.events.length} events, all ready`);
    }
    console.log(`seed: done in ${Date.now() - started} ms`);
  } catch (err) {
    console.error(`seed: failed: ${(err as Error).message}`);
    process.exitCode = 1;
  } finally {
    await pool.end();
  }
}

void main();
