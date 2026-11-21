import { Link, useLocation } from "@tanstack/react-router"
import { MenuIcon } from "lucide-react"

import { GitHubIcon } from "@/components/icons"
import { LanguageSwitcher } from "@/components/language-switcher"
import { Wordmark } from "@/components/logo"
import { Search } from "@/components/search"
import { ThemeToggle } from "@/components/theme-toggle"
import { Button, buttonVariants } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { bookOf, slugOfPath } from "@/lib/docs"
import { useLangParams, useT } from "@/lib/i18n"
import { stripLang } from "@/lib/locale"
import { site } from "@/lib/site"

const navLink = "text-muted-foreground transition-colors hover:text-foreground data-current:text-foreground"
const mobileNavLink = "min-h-11 px-3 data-current:bg-accent aria-[current=page]:bg-accent"

export function SiteHeader() {
  const t = useT()
  const lp = useLangParams()
  // The docs and native UI's are apart, each with its own sidebar.
  const book = useLocation({ select: (l) => (stripLang(l.pathname).startsWith("/docs") ? bookOf(slugOfPath(l.pathname)) : undefined) })
  return (
    <header className="frame sticky top-0 z-40 flex h-14 items-center gap-6 border-b bg-background/80 px-4 backdrop-blur-xl sm:px-6">
      <Link to="/{-$lang}" params={lp}>
        <Wordmark />
      </Link>
      <nav className="hidden items-center gap-5 text-sm sm:flex">
        <Link to="/{-$lang}/docs" params={lp} className={navLink} activeOptions={{ exact: true }} data-current={book === "docs" || undefined}>
          {t("Docs")}
        </Link>
        <Link to="/{-$lang}/docs/$" params={{ ...lp, _splat: "ui" }} className={navLink} activeOptions={{ exact: true }} data-current={book === "ui" || undefined}>
          UI
        </Link>
        <a href={`${site.repo}/tree/main/examples`} className={navLink}>
          {t("Examples")}
        </a>
        <Link to="/{-$lang}/benchmarks" params={lp} className={navLink}>
          {t("Benchmarks")}
        </Link>
        <Link to="/{-$lang}/showcase" params={lp} className={navLink}>
          {t("Showcase")}
        </Link>
      </nav>
      <div className="ml-auto flex items-center gap-1">
        <Search />
        <a href={site.repo} aria-label="GitHub" className={buttonVariants({ variant: "ghost", size: "icon" })}>
          <GitHubIcon />
        </a>
        <LanguageSwitcher />
        <ThemeToggle />
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger render={<Button variant="ghost" size="icon" className="sm:hidden" aria-label={t("Navigation menu")} />}>
            <MenuIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-44 sm:hidden">
            <DropdownMenuItem
              className={mobileNavLink}
              render={<Link to="/{-$lang}/docs" params={lp} activeOptions={{ exact: true }} data-current={book === "docs" || undefined} />}
            >
              {t("Docs")}
            </DropdownMenuItem>
            <DropdownMenuItem
              className={mobileNavLink}
              render={<Link to="/{-$lang}/docs/$" params={{ ...lp, _splat: "ui" }} activeOptions={{ exact: true }} data-current={book === "ui" || undefined} />}
            >
              UI
            </DropdownMenuItem>
            <DropdownMenuItem className={mobileNavLink} render={<a href={`${site.repo}/tree/main/examples`} />}>
              {t("Examples")}
            </DropdownMenuItem>
            <DropdownMenuItem className={mobileNavLink} render={<Link to="/{-$lang}/benchmarks" params={lp} />}>
              {t("Benchmarks")}
            </DropdownMenuItem>
            <DropdownMenuItem className={mobileNavLink} render={<Link to="/{-$lang}/showcase" params={lp} />}>
              {t("Showcase")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  )
}
