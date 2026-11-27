// Small form primitives in the shadcn style (U5): label, input, select, textarea, checkbox.
import * as React from "react";
import { cn } from "@/lib/utils";

const control =
  "w-full min-w-0 rounded-md border border-input bg-background px-2.5 py-1.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive disabled:opacity-60";

export function Input({ className, ...props }: React.ComponentProps<"input">) {
  return <input className={cn(control, className)} {...props} />;
}

export function Select({ className, ...props }: React.ComponentProps<"select">) {
  return <select className={cn(control, "pr-8", className)} {...props} />;
}

export function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return <textarea className={cn(control, "min-h-20", className)} {...props} />;
}

export function Label({ className, ...props }: React.ComponentProps<"label">) {
  return <label className={cn("grid gap-1 text-sm font-medium", className)} {...props} />;
}

export function Check({ label, className, ...props }: React.ComponentProps<"input"> & { label: React.ReactNode }) {
  return (
    <label className={cn("inline-flex items-center gap-2 text-sm", className)}>
      <input type="checkbox" className="size-4 accent-[var(--primary)]" {...props} />
      <span>{label}</span>
    </label>
  );
}

export function Hint({ className, ...props }: React.ComponentProps<"p">) {
  return <p className={cn("text-xs font-normal text-muted-foreground", className)} {...props} />;
}

export function Card({ className, ...props }: React.ComponentProps<"section">) {
  return <section className={cn("min-w-0 space-y-3 rounded-lg border p-4", className)} {...props} />;
}
