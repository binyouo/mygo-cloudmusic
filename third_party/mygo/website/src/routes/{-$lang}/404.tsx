import { createFileRoute } from "@tanstack/react-router"

import { NotFound } from "@/components/not-found"
import { translate, useNotFoundLanguage } from "@/lib/i18n"
import { langOfMatches, type Language } from "@/lib/locale"
import { notFoundLanguage, notFoundScript } from "@/lib/not-found"

// Prerendered as 404.html, which Cloudflare serves for paths without a page.
export const Route = createFileRoute("/{-$lang}/404")({
  head: ({ matches }) => ({
    meta: [{ title: `${translate((notFoundLanguage() as Language | undefined) ?? langOfMatches(matches), "Page not found")} · MyGo` }],
    scripts: [{ children: notFoundScript }],
  }),
  component: NotFoundRoute,
})

function NotFoundRoute() {
  return <NotFound lang={useNotFoundLanguage()} />
}
