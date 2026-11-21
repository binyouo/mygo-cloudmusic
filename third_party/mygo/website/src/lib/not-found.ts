declare global {
  interface Window {
    /** The path that 404.html is served at. */
    __notFoundPath?: string
    /** The language of that path: "zh" under /zh. */
    __notFoundLang?: string
  }
}

/** Run by 404.html and by /zh/404 before the page hydrates. */
export const notFoundScript = 'window.__notFoundPath=location.pathname;window.__notFoundLang=location.pathname==="/zh"||location.pathname.indexOf("/zh/")===0?"zh":"en"'

/**
 * The language of a 404 page: that of the path it is served at, which the
 * router reads as /404 (or /zh/404) once it hydrates.
 */
export function notFoundLanguage() {
  if (typeof window === "undefined") return undefined
  return window.__notFoundLang
}
