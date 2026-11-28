"use client";
// Countdown island: renders a placeholder on the server and ticks in the browser (no hydration
// mismatch, since the time is read only after mounting).
import { useEffect, useState } from "react";

type Labels = { days: string; hours: string; minutes: string; seconds: string; started: string };

export function Countdown({ target, labels }: { target: string; labels: Labels }) {
  const [now, setNow] = useState<number | null>(null);
  useEffect(() => {
    const tick = () => setNow(Date.now());
    const first = setTimeout(tick, 0);
    const id = setInterval(tick, 1000);
    return () => {
      clearTimeout(first);
      clearInterval(id);
    };
  }, []);
  const end = Date.parse(target);
  if (Number.isNaN(end)) return null;
  const left = now === null ? null : Math.max(0, end - now);
  if (left === 0) return <p className="text-xl font-semibold">{labels.started}</p>;
  const parts = [
    [labels.days, left === null ? null : Math.floor(left / 86_400_000)],
    [labels.hours, left === null ? null : Math.floor(left / 3_600_000) % 24],
    [labels.minutes, left === null ? null : Math.floor(left / 60_000) % 60],
    [labels.seconds, left === null ? null : Math.floor(left / 1000) % 60],
  ] as const;
  return (
    <div className="grid grid-cols-4 gap-2 text-center sm:max-w-lg" role="timer" aria-live="off">
      {parts.map(([label, v]) => (
        <div key={label} className="rounded-[var(--eps-radius)] bg-[var(--eps-primary)] p-3 text-[var(--eps-on-primary)]">
          <span className="block font-[family-name:var(--eps-font-heading)] text-3xl font-bold tabular-nums">{v === null ? "–" : v}</span>
          <span className="text-xs">{label}</span>
        </div>
      ))}
    </div>
  );
}
