import { useEffect, useMemo, useState, type ReactNode } from "react";

import {
  ThemeContext,
  type Theme,
  type ThemeContextValue,
} from "@/components/shared/theme-context";

const STORAGE_KEY = "teka-theme";

function systemTheme(): "dark" | "light" {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

// localStorage can throw (privacy modes, blocked storage); a theme preference
// is never worth crashing over, and this provider sits above the ErrorBoundary.
function readStoredTheme(): Theme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return stored === "dark" || stored === "light" || stored === "system" ? stored : "system";
  } catch {
    return "system";
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readStoredTheme);
  const resolvedTheme = theme === "system" ? systemTheme() : theme;

  useEffect(() => {
    const root = document.documentElement;
    root.classList.remove("light", "dark");
    root.classList.add(resolvedTheme);
  }, [resolvedTheme]);

  const value = useMemo<ThemeContextValue>(
    () => ({
      theme,
      resolvedTheme,
      setTheme: (next) => {
        try {
          localStorage.setItem(STORAGE_KEY, next);
        } catch {
          // Preference just won't survive a reload.
        }
        setThemeState(next);
      },
    }),
    [theme, resolvedTheme],
  );

  return <ThemeContext value={value}>{children}</ThemeContext>;
}
