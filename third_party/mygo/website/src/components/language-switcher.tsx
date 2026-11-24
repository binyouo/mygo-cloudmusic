import { useRouter, useRouterState } from "@tanstack/react-router"
import { LanguagesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { choiceOf, setChoice, useChoice, useLanguage, useT, type Choice } from "@/lib/i18n"
import { languages, localePath } from "@/lib/locale"

/** The same page in the language chosen: its path under /zh, or without it. */
export function LanguageSwitcher() {
  const t = useT()
  const choice = useChoice()
  const lang = useLanguage()
  const router = useRouter()
  const { pathname, searchStr, hash } = useRouterState({ select: (state) => state.location })
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="icon" aria-label={t("Language")} />}>
        <LanguagesIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        <DropdownMenuRadioGroup
          value={choice}
          onValueChange={(value) => {
            const next = value as Choice
            setChoice(next)
            const target = choiceOf(next)
            if (target !== lang) void router.navigate({ href: localePath(target, pathname) + searchStr + (hash ? `#${hash}` : "") })
          }}
        >
          <DropdownMenuRadioItem value="auto">{t("Auto")}</DropdownMenuRadioItem>
          {languages.map((language) => (
            <DropdownMenuRadioItem key={language.id} value={language.id}>
              {language.name}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
