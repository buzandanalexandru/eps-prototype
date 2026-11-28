"use client";
// The admission dial (step 5, E4): a slider and the exact number, kept together; the form posts one
// value. A save takes effect at the gateway within 2 seconds.
import { useState } from "react";
import { Input } from "@/components/ui/field";

export function RateDial({ name, defaultValue, label }: { name: string; defaultValue: number; label: string }) {
  const [v, setV] = useState(defaultValue);
  const clamp = (n: number) => Math.min(60000, Math.max(10, Number.isFinite(n) ? Math.round(n) : 10));
  return (
    <span className="flex flex-wrap items-center gap-3">
      <input type="range" min={10} max={60000} step={10} value={v} onChange={(e) => setV(clamp(Number(e.target.value)))} className="min-w-0 flex-1 accent-[var(--primary)]" aria-label={label} data-testid="sale-rate-dial" />
      <Input type="number" name={name} min={10} max={60000} value={v} onChange={(e) => setV(Number(e.target.value))} onBlur={() => setV(clamp(v))} className="w-28" aria-label={label} data-testid="sale-rate" />
    </span>
  );
}
