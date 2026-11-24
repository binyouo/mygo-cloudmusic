import { useRouterState } from "@tanstack/react-router"
import * as React from "react"

import { langOf, languages, type Language } from "@/lib/locale"
import { notFoundLanguage } from "@/lib/not-found"
import { zh } from "@/lib/zh"

/** What the reader chose: a language, or "auto" for the first of their browser's languages that the site has. */
export type Choice = Language | "auto"

const key = "language"
const listeners = new Set<() => void>()

function stored(): Choice {
  try {
    return languages.find((l) => l.id === localStorage.getItem(key))?.id ?? "auto"
  } catch {
    return "auto"
  }
}

/** The first of the browser's languages that the site has, else English. */
export function detectLanguage(): Language {
  for (const tag of navigator.languages) {
    const primary = tag.split("-")[0]!.toLowerCase()
    const found = languages.find((l) => l.id === primary)
    if (found) return found.id
  }
  return "en"
}

/** The language a choice stands for: the one detected, for "auto". */
export function choiceOf(choice: Choice): Language {
  return choice === "auto" ? detectLanguage() : choice
}

export function setChoice(choice: Choice) {
  try {
    if (choice === "auto") localStorage.removeItem(key)
    else localStorage.setItem(key, choice)
  } catch {}
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** What the reader chose: "auto" until the page hydrates. */
export function useChoice(): Choice {
  return React.useSyncExternalStore(subscribe, stored, () => "auto")
}

/** The language of the page, which its path says: /zh is Chinese. */
export function useLanguage(): Language {
  return useRouterState({ select: (state) => langOf(state.location.pathname) })
}

/**
 * The language of 404.html: that of the path it was served at, which the
 * router rewrites as /404, or /zh/404 under /zh. Its HTML is prerendered in
 * English, so the first render must be.
 */
export function useNotFoundLanguage(): Language | "auto" {
  const lang = useLanguage()
  const served = React.useState(notFoundLanguage)[0]
  return served === "zh" ? lang : "auto"
}

/** The `lang` param of links to the site's pages, which is "zh" on the pages in Chinese. */
export function useLangParams() {
  return { lang: useLanguage() === "zh" ? ("zh" as const) : undefined }
}

/**
 * Says text, written in English, in a language: `{name}` in it is replaced
 * by the value of that name in values.
 */
export function translate(lang: Language, text: string, values?: Record<string, string | number>) {
  let s = lang === "en" ? text : (zh[text] ?? text)
  if (values) s = s.replace(/\{(\w+)\}/g, (match, name: string) => (name in values ? String(values[name]) : match))
  return s
}

/** The translator of the language of the page, or of another language. Components that use it render again when the page's language changes. */
export function useT(language?: Language | "auto") {
  const lang = useLanguage()
  const said = language === undefined || language === "auto" ? lang : language
  return React.useCallback((text: string, values?: Record<string, string | number>) => translate(said, text, values), [said])
}
