// The published platform fee for public pages (read without sign-in, platform table).
import "server-only";
import { getAppPool, hasDatabase } from "../db/pool";
import { withTx } from "../db/tx";
import { getPlatformFee } from "../organisers/domain";
import type { PlatformFee } from "../pricing/fees";

/** The value published at launch (migration 00005); used only when no database is configured. */
export const LAUNCH_FEE: PlatformFee = { percentBps: 250, fixedMinor: 0 };

export async function publishedFee(): Promise<{ fee: PlatformFee; live: boolean }> {
  if (!hasDatabase()) return { fee: LAUNCH_FEE, live: false };
  try {
    return { fee: await withTx(getAppPool(), {}, (c) => getPlatformFee(c, "RON")), live: true };
  } catch {
    return { fee: LAUNCH_FEE, live: false };
  }
}
