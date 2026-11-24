/** The languages the site is written in, each named in itself, with its BCP 47 tag. */
export const languages = [
  { id: "en", name: "English", tag: "en" },
  { id: "zh", name: "简体中文", tag: "zh-Hans" },
] as const

export type Language = (typeof languages)[number]["id"]

/** The language a route's `lang` param names: "zh" under /zh, English otherwise. */
export const languageOf = (lang: string | undefined): Language => (lang === "zh" ? "zh" : "en")

export const tagOf = (lang: Language) => languages.find((l) => l.id === lang)!.tag

/** The language of a path: Chinese under /zh, English everywhere else. */
export function langOf(pathname: string): Language {
  return pathname === "/zh" || pathname.startsWith("/zh/") ? "zh" : "en"
}

/** The language of a page's head, from its matches, the last of which is the page's own. */
export function langOfMatches(matches: { pathname: string }[]): Language {
  return langOf(matches.at(-1)?.pathname ?? "/")
}

/** A path without its language's prefix: "/zh/docs" is "/docs", "/zh" is "/". */
export function stripLang(pathname: string) {
  if (pathname === "/zh") return "/"
  return pathname.startsWith("/zh/") ? pathname.slice(3) : pathname
}

/** A path in a language: "/docs" in Chinese is "/zh/docs". */
export function localePath(lang: Language, pathname: string) {
  const bare = stripLang(pathname)
  if (lang === "en") return bare
  return bare === "/" ? "/zh" : `/zh${bare}`
}

/**
 * Runs in <head> before the page paints: a reader whose browser asks for Chinese
 * first, or who chose it, is sent from an English page to its page under /zh.
 * A choice of English, or a page under /zh already, is left as it is.
 */
export const localeScript = `(function(){try{var p=location.pathname;if(p==="/zh"||p.indexOf("/zh/")===0)return;var c=localStorage.getItem("language");if(c==="en")return;var z=c==="zh";if(!z){var l=navigator.languages||[navigator.language];for(var i=0;i<l.length;i++){var g=l[i].split("-")[0].toLowerCase();if(g==="en")break;if(g==="zh"){z=true;break}}}if(z)location.replace("/zh"+(p==="/"?"":p)+location.search+location.hash)}catch(e){}})()`
