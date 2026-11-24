import { Link } from "@tanstack/react-router"

import { useLangParams, useT } from "@/lib/i18n"
import { site } from "@/lib/site"

export function SiteFooter() {
  const t = useT()
  const lp = useLangParams()
  return (
    <footer className="frame label flex flex-wrap items-center justify-between gap-4 border-t px-4 py-6 sm:px-6">
      <span>
        MyGo {__MYGO_VERSION__} ·{" "}
        <a href={`${site.repo}/blob/main/LICENSE`} className="hover:text-foreground">
          {t("MIT License")}
        </a>
      </span>
      <nav className="flex flex-wrap gap-x-6 gap-y-2">
        <Link to="/{-$lang}/docs" params={lp} className="hover:text-foreground">
          {t("Docs")}
        </Link>
        <Link to="/{-$lang}/benchmarks" params={lp} className="hover:text-foreground">
          {t("Benchmarks")}
        </Link>
        <Link to="/{-$lang}/showcase" params={lp} className="hover:text-foreground">
          {t("Showcase")}
        </Link>
        <Link to="/{-$lang}/sponsors" params={lp} className="hover:text-foreground">
          {t("Sponsors")}
        </Link>
        <a href={`${site.repo}/releases`} className="hover:text-foreground">
          {t("Releases")}
        </a>
        <a href={site.repo} className="hover:text-foreground">
          GitHub
        </a>
      </nav>
    </footer>
  )
}
