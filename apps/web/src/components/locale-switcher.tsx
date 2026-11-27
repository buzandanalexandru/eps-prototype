import { setLocale } from "@/lib/session/actions";

export function LocaleSwitcher({ current, label }: { current: string; label: string }) {
  return (
    <form action={setLocale} className="flex items-center gap-1" aria-label={label}>
      {(["ro", "en"] as const).map((l) => (
        <button
          key={l}
          name="locale"
          value={l}
          aria-pressed={current === l}
          className={`rounded px-2 py-1 uppercase ${current === l ? "bg-muted font-semibold" : ""}`}
        >
          {l}
        </button>
      ))}
    </form>
  );
}
