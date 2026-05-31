import { useEffect, useState, type ReactNode } from 'react'

import {
  applyThemeClass,
  THEME_STORAGE_KEY,
  ThemeContext,
  type Theme,
} from '@/lib/theme'

// The initial theme is resolved before React mounts by the inline script in
// index.html (which sets the `dark` class to avoid a flash). We read it back
// from <html> here so the two never disagree.
function getInitialTheme(): Theme {
  if (typeof document === 'undefined') return 'dark'
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(getInitialTheme)

  useEffect(() => {
    applyThemeClass(theme)
    try {
      localStorage.setItem(THEME_STORAGE_KEY, theme)
    } catch {
      // Ignore storage failures (e.g. private browsing).
    }
  }, [theme])

  return (
    <ThemeContext.Provider
      value={{
        theme,
        setTheme,
        toggleTheme: () =>
          setTheme((current) => (current === 'dark' ? 'light' : 'dark')),
      }}
    >
      {children}
    </ThemeContext.Provider>
  )
}
