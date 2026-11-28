"use client";
// Asks the server for the page again every few seconds while a core read failed, so a page that
// could not reach the core under load recovers by itself (step 6 review).
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export function RetryRefresh({ everyMs = 5000 }: { everyMs?: number }) {
  const router = useRouter();
  useEffect(() => {
    const id = setInterval(() => router.refresh(), everyMs);
    return () => clearInterval(id);
  }, [router, everyMs]);
  return null;
}
