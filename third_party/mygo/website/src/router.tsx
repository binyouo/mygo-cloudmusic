import { createRouter } from "@tanstack/react-router"

import { langOf } from "@/lib/locale"
import { notFoundLanguage } from "@/lib/not-found"
import { routeTree } from "./routeTree.gen"

export function getRouter() {
  // 404.html is the page of /404 (of /zh/404 under /zh), served at any path
  // without a page: the router reads that path as the 404 page, so that the
  // page hydrates as rendered, and still shows the path in the address bar.
  const notFound = typeof window === "undefined" ? undefined : window.__notFoundPath
  const lang = langOf(notFound ?? "/")
  const page = notFound && (lang === "zh" ? "/zh/404" : "/404")
  // The 404 page is in the language of that path, which its head and it read
  // until the reader goes elsewhere.
  if (notFoundLanguage() !== undefined) window.__notFoundLang = lang
  return createRouter({
    routeTree,
    scrollRestoration: true,
    defaultPreload: "intent",
    rewrite: notFound
      ? {
          input: ({ url }) => {
            if (url.pathname === notFound) url.pathname = page!
            return url
          },
          output: ({ url }) => {
            if (url.pathname === page) url.pathname = notFound
            return url
          },
        }
      : undefined,
  })
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof getRouter>
  }
}
