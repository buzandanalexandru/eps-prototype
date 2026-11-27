"use client";
// A form bound to a server action with useActionState: shows the saved message, field errors next
// to fields (FieldError) and every other error in a summary, all translated from error codes.
import { createContext, startTransition, useActionState, useContext, useMemo, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import type { FormState } from "@/lib/studio/form";
import { cn } from "@/lib/utils";

type Ctx = { state: FormState; rendered: Set<string> };
const FormCtx = createContext<Ctx>({ state: {}, rendered: new Set() });

export function useFormResult() {
  return useContext(FormCtx).state;
}

export function ActionForm({
  action,
  children,
  submit,
  className,
  variant = "default",
  testId,
  savedKey = "saved",
}: {
  action: (prev: FormState, fd: FormData) => Promise<FormState>;
  children?: ReactNode;
  submit: string;
  className?: string;
  variant?: "default" | "outline" | "destructive" | "secondary";
  testId?: string;
  savedKey?: string;
}) {
  const [state, formAction, pending] = useActionState(action, {});
  // FieldErrors rendered inside this form register their names here (children render before the
  // summary), so the summary lists only errors that have no place next to a field.
  const rendered = useMemo(() => ({ state, names: new Set<string>() }), [state]);
  return (
    <FormCtx value={{ state, rendered: rendered.names }}>
      <form
        action={formAction}
        // Submitting through a transition (instead of the form action) keeps what the user typed when
        // the server refuses it: React resets a form after an action submitted through `action`.
        onSubmit={(e) => {
          e.preventDefault();
          const fd = new FormData(e.currentTarget, (e.nativeEvent as SubmitEvent).submitter);
          startTransition(() => formAction(fd));
        }}
        className={cn("min-w-0 space-y-3", className)}
        data-testid={testId}
        noValidate
      >
        {children}
        <ErrorSummary />
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" variant={variant} disabled={pending}>
            {submit}
          </Button>
          {state.ok && <Saved k={state.message ?? savedKey} />}
        </div>
      </form>
    </FormCtx>
  );
}

function Saved({ k }: { k: string }) {
  const t = useTranslations("studio.common");
  return (
    <span role="status" className="text-sm text-emerald-700 dark:text-emerald-400" data-testid="saved">
      {t.has(k) ? t(k) : t("saved")}
    </span>
  );
}

export function useErrorText() {
  const t = useTranslations("errors");
  return (code: string, params?: Record<string, string | number>) => (t.has(code) ? t(code, params as Record<string, string>) : t("invalid"));
}

/** The error for one field, shown next to it. */
export function FieldError({ name }: { name: string }) {
  const { state, rendered } = useContext(FormCtx);
  rendered.add(name);
  const text = useErrorText();
  const code = state.errors?.[name];
  if (!code) return null;
  return (
    <span role="alert" className="text-xs font-normal text-destructive" data-testid={`error-${name}`}>
      {text(code, state.params)}
    </span>
  );
}

function ErrorSummary() {
  const { state, rendered } = useContext(FormCtx);
  const text = useErrorText();
  const rest = Object.entries(state.errors ?? {}).filter(([k]) => !rendered.has(k));
  if (!rest.length) return null;
  return (
    <ul role="alert" className="space-y-1 rounded-md border border-destructive/40 p-2 text-sm text-destructive" data-testid="form-errors">
      {rest.map(([k, code]) => (
        <li key={k}>{text(code, state.params)}</li>
      ))}
    </ul>
  );
}
