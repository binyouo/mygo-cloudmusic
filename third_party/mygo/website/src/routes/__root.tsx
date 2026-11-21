import { HeadContent, Outlet, Scripts, createRootRoute } from "@tanstack/react-router"
import * as React from "react"

import { NotFound } from "@/components/not-found"
import { SiteFooter } from "@/components/site-footer"
import { SiteHeader } from "@/components/site-header"
import { translate, useLanguage } from "@/lib/i18n"
import { langOfMatches, localeScript, tagOf } from "@/lib/locale"
import { site } from "@/lib/site"
import { themeScript } from "@/lib/theme"
import appCss from "@/styles.css?url"

export const Route = createRootRoute({
  head: ({ matches }) => {
    const lang = langOfMatches(matches)
    const t = (text: string) => translate(lang, text)
    return {
      meta: [
        { charSet: "utf-8" },
        { name: "viewport", content: "width=device-width, initial-scale=1" },
        { title: t(site.title) },
        { name: "description", content: t(site.description) },
        { property: "og:type", content: "website" },
        { property: "og:site_name", content: site.name },
        { property: "og:title", content: t(site.title) },
        { property: "og:description", content: t(site.description) },
        { name: "twitter:card", content: "summary" },
      ],
      links: [
        { rel: "stylesheet", href: appCss },
        { rel: "icon", href: "/favicon.svg", type: "image/svg+xml" },
      ],
      scripts: [
        { children: themeScript },
        { children: localeScript },
        ...(import.meta.env.PROD
          ? [
              {
                src: "https://tt.egoist.dev/js/script.js",
                defer: true,
                "data-domain": "mygo.run",
              },
            ]
          : []),
      ],
    }
  },
  shellComponent: RootDocument,
  component: RootLayout,
  notFoundComponent: RootNotFound,
})

function RootDocument({ children }: { children: React.ReactNode }) {
  const lang = useLanguage()
  return (
    <html lang={tagOf(lang)} suppressHydrationWarning>
      <head>
        <HeadContent />
      </head>
      <body>
        {children}
        <Scripts />
      </body>
    </html>
  )
}

/** A path without a page in neither language: its language is the path's own. */
function RootNotFound() {
  return <NotFound />
}

function RootLayout() {
  return (
    <div className="flex min-h-svh flex-col">
      <SiteHeader />
      <div className="frame flex flex-1 flex-col">
        <Outlet />
      </div>
      <SiteFooter />
    </div>
  )
}
