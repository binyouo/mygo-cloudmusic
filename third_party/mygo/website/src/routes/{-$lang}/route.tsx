import { Outlet, createFileRoute, notFound } from "@tanstack/react-router"

// The pages in English, at their paths, and in Chinese under /zh: `lang` is
// that optional "zh" prefix, and no other segment is a language.
export const Route = createFileRoute("/{-$lang}")({
  beforeLoad: ({ params }) => {
    if (params.lang !== undefined && params.lang !== "zh") throw notFound()
  },
  component: Outlet,
})
