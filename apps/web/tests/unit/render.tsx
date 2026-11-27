import type { ReactElement } from "react";
import { renderToString } from "react-dom/server";
import { NextIntlClientProvider, type AbstractIntlMessages } from "next-intl";

/** Server-renders an element inside next-intl, as a page would be. */
export function renderWithIntl(locale: string, messages: AbstractIntlMessages, element: ReactElement): string {
  return renderToString(
    <NextIntlClientProvider locale={locale} messages={messages}>
      {element}
    </NextIntlClientProvider>,
  );
}
