import { Link, type LinkComponentProps } from "@tanstack/react-router"

import { useLangParams } from "@/lib/i18n"

/**
 * A link to a page of the docs by its slug, "" for the introduction, in the
 * language of the page. Only the page itself is active: /docs/plugins is not
 * when /docs/plugins/fetch is.
 */
export function DocLink({ slug, ...props }: { slug: string } & Omit<LinkComponentProps<"a">, "to" | "params">) {
  const lp = useLangParams()
  return slug ? (
    <Link to="/{-$lang}/docs/$" params={{ ...lp, _splat: slug }} activeOptions={{ exact: true }} {...props} />
  ) : (
    <Link to="/{-$lang}/docs" params={lp} activeOptions={{ exact: true }} {...props} />
  )
}
