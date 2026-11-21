import { Link } from "@tanstack/react-router"

import { buttonVariants } from "@/components/ui/button"
import { useLangParams, useT } from "@/lib/i18n"
import type { Language } from "@/lib/locale"

/** The page of a 404, in a language: the 404.html served at a path says it in that path's. */
export function NotFound({ lang }: { lang?: Language | "auto" }) {
  const t = useT(lang)
  const lp = useLangParams()
  return (
    <main className="flex flex-1 flex-col items-center justify-center px-6 py-32 text-center">
      <p className="font-mono text-sm text-muted-foreground">404</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight">{t("This page does not exist")}</h1>
      <p className="mt-3 max-w-sm text-muted-foreground">
        {t("It may have moved. The docs, or their search, will have what you are looking for.")}
      </p>
      <div className="mt-8 flex gap-3">
        <Link to="/{-$lang}/docs" params={lp} className={buttonVariants({ size: "lg" })}>
          {t("Read the docs")}
        </Link>
        <Link to="/{-$lang}" params={lp} className={buttonVariants({ variant: "outline", size: "lg" })}>
          {t("Home")}
        </Link>
      </div>
    </main>
  )
}
