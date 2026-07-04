import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";

export type ThemeMode = "dark" | "light";

interface ThemeContextValue {
  mode: ThemeMode;
  setTheme: (mode: ThemeMode) => void;
  toggle: () => void;
}

const ThemeContext = createContext<ThemeContextValue>({
  mode: "dark",
  setTheme: () => {},
  toggle: () => {},
});

const STORAGE_KEY = "bc-theme";

const LABELS: Record<ThemeMode, string> = {
  dark: "Dark",
  light: "Light",
};

function readStored(): ThemeMode {
  try {
    const val = localStorage.getItem(STORAGE_KEY);
    if (val === "dark" || val === "light") return val;
    // Retired modes ("solar-flare", "system") map to the dark default.
  } catch {
    // localStorage unavailable
  }
  return "dark";
}

function applyTheme(mode: ThemeMode) {
  const el = document.documentElement;
  // Dark is the :root default; light is the only theme class.
  el.classList.remove("dark", "light");
  if (mode === "light") {
    el.classList.add("light");
  }
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [mode, setMode] = useState<ThemeMode>(readStored);

  useEffect(() => {
    applyTheme(mode);
    try {
      localStorage.setItem(STORAGE_KEY, mode);
    } catch {
      // ignore
    }
  }, [mode]);

  const setTheme = useCallback((m: ThemeMode) => {
    setMode(m);
  }, []);

  const toggle = useCallback(() => {
    setMode((prev) => (prev === "dark" ? "light" : "dark"));
  }, []);

  return (
    <ThemeContext.Provider value={{ mode, setTheme, toggle }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  return useContext(ThemeContext);
}

export { LABELS as THEME_LABELS };
