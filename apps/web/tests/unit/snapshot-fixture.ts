// The shared hand-written snapshot (services/internal/snapshot/testdata/galaxia.v1.json), for web tests.
import { readFileSync } from "node:fs";
import path from "node:path";
import type { SaleSnapshot } from "@/lib/snapshot/types";

export function galaxiaSnapshot(): SaleSnapshot {
  return JSON.parse(readFileSync(path.resolve(__dirname, "../../../../services/internal/snapshot/testdata/galaxia.v1.json"), "utf8"));
}
export const GA = "0d000000-0000-4000-8000-000000000001";
export const CAMPING = "0d000000-0000-4000-8000-000000000002";
