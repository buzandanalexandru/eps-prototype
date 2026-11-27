import "server-only";
import { headers } from "next/headers";
import { hasDatabase } from "../db/pool";
import { getAuth } from "./server";

export async function currentSession() {
  if (!hasDatabase()) return null;
  try {
    return await getAuth().api.getSession({ headers: await headers(), query: { disableCookieCache: true } });
  } catch {
    return null;
  }
}
