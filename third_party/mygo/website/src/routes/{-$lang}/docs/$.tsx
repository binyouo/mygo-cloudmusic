import { createFileRoute } from "@tanstack/react-router"

import { DocPage, docHead, loadDocPage } from "@/components/docs/doc-page"
import { NotFound } from "@/components/not-found"
import { languageOf } from "@/lib/locale"

// Every page but the introduction, nested ones too: /docs/plugins/fetch.
export const Route = createFileRoute("/{-$lang}/docs/$")({
  loader: ({ params, parentMatchPromise }) => loadDocPage(params._splat ?? "", languageOf(params.lang), parentMatchPromise),
  head: ({ loaderData }) => docHead(loaderData),
  component: DocRoute,
})

function DocRoute() {
  const doc = Route.useLoaderData()
  return doc ? <DocPage doc={doc} /> : <NotFound />
}
