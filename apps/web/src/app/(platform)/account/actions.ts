"use server";
// Delete my data (fan-account capability, GDPR): first the person's sales data in every organiser's
// shop (the core anonymises what the law requires to keep), then the account itself in the control
// schema (user, sessions, sign-in accounts). An organiser team member must leave the team first.
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { getAuth } from "@/lib/auth/server";
import { currentSession } from "@/lib/auth/session";
import { getAppPool } from "@/lib/db/pool";
import { withTx } from "@/lib/db/tx";
import { fanCore } from "@/lib/fan/core";

export async function deleteMyDataAction(form: FormData) {
  if (form.get("confirm") !== "yes") redirect("/account?error=confirm");
  const s = await currentSession();
  if (!s) redirect("/sign-in?next=/account");
  const member = await withTx(getAppPool(), { userId: s.user.id }, async (c) => (await c.query("select 1 from control.memberships where user_id = $1 limit 1", [s.user.id])).rowCount);
  if (member) redirect("/account?error=member");
  const r = await fanCore<{ orders: number }>("POST", "/v1/me/erase", {});
  if (!r.ok) redirect(`/account?error=${encodeURIComponent(r.code)}`);
  const auth = getAuth();
  // Signed out first (the session cookie goes), then every session, sign-in account and the user.
  await auth.api.signOut({ headers: await headers() }).catch(() => undefined);
  const ctx = await auth.$context;
  await ctx.internalAdapter.deleteUserSessions(s.user.id);
  await ctx.internalAdapter.deleteAccounts(s.user.id);
  await ctx.internalAdapter.deleteUser(s.user.id);
  redirect("/?deleted=1");
}
