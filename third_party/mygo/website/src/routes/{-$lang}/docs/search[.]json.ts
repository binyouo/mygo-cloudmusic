import { createFileRoute } from "@tanstack/react-router"

import { loadSearchIndex } from "@/lib/docs.server"
import { langOf } from "@/lib/locale"

// Prerendered to /docs/search.json and /zh/docs/search.json, which the
// search of the site fetches, each in its language.
export const Route = createFileRoute("/{-$lang}/docs/search.json")({
  server: {
    handlers: {
      GET: async ({ request }) => Response.json(await loadSearchIndex(langOf(new URL(request.url).pathname))),
    },
  },
})
