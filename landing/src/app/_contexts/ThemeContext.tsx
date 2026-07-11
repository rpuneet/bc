"use client";

import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  ReactNode,
} from "react";

type Theme = "light" | "dark" | "system";

interface ThemeContextType {
  theme: Theme;
  resolvedTheme: "light" | "dark";
  toggleTheme: () => void;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

const STORAGE_KEY = "mycel-theme";

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>("dark");
  const [resolvedTheme, setResolvedTheme] = useState<"light" | "dark">("dark");
  const [mounted, setMounted] = useState(false);

  const applyTheme = (themeName: "light" | "dark") => {
    const html = document.documentElement;
    if (themeName === "dark") {
      html.classList.add("dark");
    } else {
      html.classList.remove("dark");
    }
  };

  // Initialize theme
  useEffect(() => {
    const initializeTheme = () => {
      // Get stored preference
      const stored = localStorage.getItem(STORAGE_KEY) as Theme | null;
      const preferredTheme = stored || "dark";

      setThemeState(preferredTheme);

      // Determine actual theme
      let actualTheme: "light" | "dark" = "light";
      if (preferredTheme === "system") {
        actualTheme = window.matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light";
      } else {
        actualTheme = preferredTheme === "dark" ? "dark" : "light";
      }

      setResolvedTheme(actualTheme);
      applyTheme(actualTheme);
      setMounted(true);
    };

    // Use setTimeout to ensure DOM is ready
    setTimeout(initializeTheme, 0);
  }, []);

  // Listen for system preference changes
  useEffect(() => {
    if (!mounted || theme !== "system") return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = (e: MediaQueryListEvent) => {
      const newTheme = e.matches ? "dark" : "light";
      setResolvedTheme(newTheme);
      applyTheme(newTheme);
    };

    mediaQuery.addEventListener("change", handleChange);
    return () => mediaQuery.removeEventListener("change", handleChange);
  }, [mounted, theme]);

  const setTheme = (newTheme: Theme) => {
    setThemeState(newTheme);
    localStorage.setItem(STORAGE_KEY, newTheme);

    // Apply theme
    let actualTheme: "light" | "dark" = "light";
    if (newTheme === "system") {
      actualTheme = window.matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light";
    } else {
      actualTheme = newTheme === "dark" ? "dark" : "light";
    }

    setResolvedTheme(actualTheme);
    applyTheme(actualTheme);

    // Dispatch event for analytics
    window.dispatchEvent(
      new CustomEvent("themechange", { detail: { theme: actualTheme } }),
    );
  };

  const toggleTheme = () => {
    const newTheme = resolvedTheme === "light" ? "dark" : "light";
    setTheme(newTheme);
  };

  return (
    <ThemeContext.Provider
      value={{ theme, resolvedTheme, toggleTheme, setTheme }}
    >
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error("useTheme must be used within ThemeProvider");
  }
  return context;
}
