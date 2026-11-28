"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { authClient } from "@/lib/auth/client";

type Props = { next: string; google: boolean; demo: boolean; initialEmail: string };

export function SignInForm({ next, google, demo, initialEmail }: Props) {
  const t = useTranslations("signIn");
  const [email, setEmail] = useState(initialEmail);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function sendLink(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const errorURL = `/sign-in?next=${encodeURIComponent(next)}`;
    const { error } = await authClient.signIn.magicLink({ email, callbackURL: next, errorCallbackURL: errorURL });
    setBusy(false);
    if (error) {
      setError(t("sendFailed"));
      return;
    }
    // A full navigation, so the server renders the demo mailbox for this address.
    // eslint-disable-next-line @next/next/no-location-assign-relative-destination
    window.location.assign(`/sign-in?next=${encodeURIComponent(next)}&sent=${encodeURIComponent(email)}`);
  }

  async function demoSignIn(as: "organiser" | "fan") {
    setBusy(true);
    setError(null);
    const res = await fetch("/api/auth/demo/sign-in", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ as }),
    });
    if (!res.ok) {
      setBusy(false);
      setError(t("demoFailed"));
      return;
    }
    // A full navigation, so every server component sees the new session cookie.
    window.location.assign(as === "organiser" ? next : next === "/studio" ? "/status" : next);
  }

  return (
    <div className="space-y-6">
      <form onSubmit={sendLink} className="space-y-3">
        <label className="block text-sm font-medium" htmlFor="email">
          {t("email")}
        </label>
        <input
          id="email"
          name="email"
          type="email"
          required
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full max-w-md rounded-md border bg-background px-3 py-2"
        />
        <div>
          <Button type="submit" disabled={busy}>
            {busy ? t("sending") : t("sendLink")}
          </Button>
        </div>
      </form>

      {google && (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">{t("or")}</p>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => authClient.signIn.social({ provider: "google", callbackURL: next })}
          >
            {t("google")}
          </Button>
        </div>
      )}

      {demo && (
        <section className="space-y-2 rounded-lg border p-4" aria-labelledby="demo-title">
          <h2 id="demo-title" className="font-semibold">
            {t("demoTitle")}
          </h2>
          <p className="text-sm text-muted-foreground">{t("demoLead")}</p>
          <div className="flex flex-wrap gap-2">
            <Button disabled={busy} onClick={() => demoSignIn("organiser")} data-testid="demo-organiser">
              {t("demoOrganiser")}
            </Button>
            <Button variant="outline" disabled={busy} onClick={() => demoSignIn("fan")} data-testid="demo-fan">
              {t("demoFan")}
            </Button>
          </div>
        </section>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
