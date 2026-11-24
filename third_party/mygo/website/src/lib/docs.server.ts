// The docs of the repository (../docs), and their Chinese (../docs-zh), read
// and rendered on the server. The site is prerendered, so this runs at build
// time only. A page without a Chinese translation is shown in English.

import fs from "node:fs/promises"
import path from "node:path"
import type { List, Root } from "mdast"
import { toString } from "mdast-util-to-string"
import remarkParse from "remark-parse"
import { unified } from "unified"

import { bookOf, type Book, type Doc, type Navs, type NavItem, type NavSection, type SearchEntry } from "@/lib/docs"
import type { Language } from "@/lib/locale"
import { renderMarkdown, slugOf, type RenderedMarkdown } from "@/lib/markdown.server"
import { site } from "@/lib/site"

// Vite runs in website/, next to the repository's docs.
const repoDir = path.resolve(process.cwd(), "..")
const docsDir = path.join(repoDir, "docs")
const docsZhDir = path.join(repoDir, "docs-zh")

/** The directory of a language's docs: ../docs, or ../docs-zh, whose pages are the same. */
const treeOf = (lang: Language) => {
  const dir = lang === "zh" ? docsZhDir : docsDir
  return { dir, name: path.basename(dir), basePath: lang === "zh" ? "/zh/docs" : "/docs" }
}

/** The files a page may be: x.md, else x/README.md, as ui/README.md is /docs/ui. */
function filesOf(slug: string) {
  if (slug === "") return ["README.md"]
  if (!/^[a-z0-9-]+(\/[a-z0-9-]+)*$/.test(slug)) return []
  return [`${slug}.md`, `${slug}/README.md`]
}

/**
 * The file of a page in a language. Chinese reads ../docs-zh first, then the
 * English page for a page not translated yet.
 */
async function fileOf(slug: string, lang: Language) {
  const trees = lang === "zh" ? [treeOf("zh"), treeOf("en")] : [treeOf("en")]
  for (const tree of trees) {
    for (const file of filesOf(slug)) {
      if (await fs.stat(path.join(tree.dir, file)).then((s) => s.isFile(), () => false)) return { ...tree, file }
    }
  }
  return undefined
}

// Pages are rendered once per build; in development, on every request.
const cache = new Map<string, Promise<RenderedMarkdown | undefined>>()

function render(slug: string, lang: Language) {
  const key = `${lang}/${slug}`
  let page = cache.get(key)
  if (!page) {
    page = fileOf(slug, lang).then((source) => {
      if (!source) return undefined
      return fs.readFile(path.join(source.dir, source.file), "utf8").then(
        // Links stay in the reader's language, even where a page falls back to English.
        (text) =>
          renderMarkdown(text, path.join(source.dir, source.file), {
            docsDir: source.dir,
            repoDir,
            repoUrl: site.repo,
            basePath: lang === "zh" ? "/zh/docs" : "/docs",
          }),
        () => undefined
      )
    })
    if (!import.meta.env.DEV) cache.set(key, page)
  }
  return page
}

/**
 * The sidebars: the docs', and native UI's, which has its own. Each takes the
 * lists of its README, in their order, under their headings, and the pages
 * of its directory it leaves out.
 */
export async function loadNav(lang: Language = "en"): Promise<Navs> {
  const [docs, ui] = await Promise.all([
    bookNav("README.md", { title: "Overview", item: { slug: "", title: "Introduction" } }, lang),
    bookNav("ui/README.md", { title: "Native UI", item: { slug: "ui", title: "Overview" } }, lang),
  ])
  return { docs, ui }
}

async function bookNav(readmeFile: string, overview: { title: string; item: NavItem }, lang: Language): Promise<NavSection[]> {
  const tree = treeOf(lang)
  const english = treeOf("en")
  const readme = await fs
    .readFile(path.join(tree.dir, readmeFile), "utf8")
    .catch(() => fs.readFile(path.join(english.dir, readmeFile), "utf8"))
  const base = path.dirname(readmeFile)
  const parsed = unified().use(remarkParse).parse(readme) as Root
  const sections: NavSection[] = [{ title: overview.title, items: [overview.item] }]
  let heading = ""
  for (const node of parsed.children) {
    if (node.type === "heading" && node.depth === 2) heading = toString(node)
    if (node.type === "list" && heading) sections.push({ title: heading, items: navItems(node, base) })
  }

  const book = bookOf(overview.item.slug!)
  const listed = new Set(sections.flatMap((s) => s.items.map((i) => i.slug)))
  const files = (await fs.readdir(tree.dir, { recursive: true })).filter((f) => f.endsWith(".md")).sort()
  const rest: NavItem[] = []
  for (const file of files) {
    const slug = slugOf(file)
    if (listed.has(slug) || bookOf(slug) !== book) continue
    const page = await render(slug, lang)
    rest.push({ slug, title: page?.title || slug })
  }
  if (rest.length) sections.push({ title: "More", items: rest })
  return sections
}

function navItems(list: List, base: string): NavItem[] {
  return list.children.flatMap((item): NavItem[] => {
    const paragraph = item.children[0]
    if (paragraph?.type !== "paragraph") return []
    const [first, ...rest] = paragraph.children
    const description = sentence(rest.map((n) => toString(n)).join(""))
    if (first?.type === "link" && first.url.endsWith(".md") && !/^[a-z]+:/.test(first.url)) {
      return [{ slug: slugOf(path.join(base, first.url)), title: toString(first), description }]
    }
    // "The Go API: `go doc -all <package>`, …": the package on pkg.go.dev.
    const pkg = rest[0]?.type === "inlineCode" ? /^go doc (?:-all )?(\S+)$/.exec(rest[0].value)?.[1] : undefined
    if (first?.type === "text" && pkg) {
      return [{ href: `https://pkg.go.dev/${pkg}`, title: first.value.split(":")[0]!.replace(/^The /, "") }]
    }
    return []
  })
}

/** "`: install the tools, then …`" → "Install the tools, then …", and "：推送…" → "推送…". */
function sentence(text: string) {
  const s = text.replace(/^[\s:：]+/, "").replace(/\s+/g, " ").trim()
  return s.charAt(0).toUpperCase() + s.slice(1)
}

export async function loadDoc(slug: string, lang: Language = "en"): Promise<Doc | undefined> {
  const [page, nav, source] = await Promise.all([render(slug, lang), loadNav(lang), fileOf(slug, lang)])
  if (!page || !source) return undefined
  const book: Book = bookOf(slug)
  const sections = nav[book]
  const order = sections.flatMap((s) => s.items).filter((i) => i.slug !== undefined)
  const index = order.findIndex((i) => i.slug === slug)
  const item = order[index]
  const link = (i: NavItem | undefined) => (i?.slug === undefined ? undefined : { slug: i.slug, title: i.title })
  return {
    slug,
    lang,
    title: page.title,
    description: item?.description || page.lead,
    summary: item?.description,
    section: sections.find((s) => s.items.includes(item!))?.title,
    html: page.html,
    toc: page.toc,
    prev: index > 0 ? link(order[index - 1]) : undefined,
    next: index >= 0 ? link(order[index + 1]) : undefined,
    markdown: (source.basePath + (slug ? `/${slug}` : "")) + ".md",
    editUrl: `${site.repo}/edit/main/${source.name}/${source.file}`,
  }
}

/** Every heading of every page of a language, with its text, for the search of the site. */
export async function loadSearchIndex(lang: Language = "en"): Promise<SearchEntry[]> {
  const nav = await loadNav(lang)
  const entries: SearchEntry[] = []
  for (const item of [...nav.docs, ...nav.ui].flatMap((s) => s.items)) {
    if (item.slug === undefined) continue
    const page = await render(item.slug, lang)
    if (!page) continue
    for (const section of page.sections) {
      entries.push({ slug: item.slug, page: page.title, id: section.id, heading: section.heading, text: section.text })
    }
  }
  return entries
}
