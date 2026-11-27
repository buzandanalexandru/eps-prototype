import "server-only";
import { headers } from "next/headers";
import { getAuth } from "./server";

export async function currentSession() {
  if (!process.env.DATABASE_URL) return null;
  try {
    return await getAuth().api.getSession({ headers: await headers() });
  } catch {
    return null;
  }
}
