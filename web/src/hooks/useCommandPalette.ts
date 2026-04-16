import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

export interface CommandItem {
  id: string;
  label: string;
  section: "Navigate" | "Action";
  icon: string;
  action: () => void;
}

export function useCommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const navigate = useNavigate();

  const toggle = useCallback(() => {
    setOpen((prev) => {
      if (prev) setQuery("");
      return !prev;
    });
  }, []);

  const close = useCallback(() => {
    setOpen(false);
    setQuery("");
  }, []);

  // Cmd+K / Ctrl+K listener
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        toggle();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [toggle]);

  const items: CommandItem[] = useMemo(
    () => [
      // Navigation — mirrors the sidebar order exactly.
      { id: "nav-live", label: "Live", section: "Navigate", icon: "~", action: () => navigate("/live") },
      { id: "nav-agents", label: "Agents", section: "Navigate", icon: "A", action: () => navigate("/agents") },
      { id: "nav-channels", label: "Channels", section: "Navigate", icon: "C", action: () => navigate("/channels") },
      { id: "nav-code", label: "Code", section: "Navigate", icon: "<", action: () => navigate("/code") },
      { id: "nav-templates", label: "Templates", section: "Navigate", icon: "T", action: () => navigate("/templates") },
      { id: "nav-tools", label: "Tools", section: "Navigate", icon: "t", action: () => navigate("/tools") },
      { id: "nav-cron", label: "Cron", section: "Navigate", icon: "@", action: () => navigate("/cron") },
      { id: "nav-secrets", label: "Secrets", section: "Navigate", icon: "#", action: () => navigate("/secrets") },
      { id: "nav-metrics", label: "Metrics", section: "Navigate", icon: "M", action: () => navigate("/metrics") },
      { id: "nav-settings", label: "Settings", section: "Navigate", icon: "\u2699", action: () => navigate("/settings") },
      // Actions
      { id: "act-create-agent", label: "Create Agent", section: "Action", icon: "+", action: () => navigate("/agents?action=create") },
      { id: "act-create-channel", label: "Create Channel", section: "Action", icon: "+", action: () => navigate("/channels?action=create") },
    ],
    [navigate],
  );

  const filtered = useMemo(() => {
    if (!query.trim()) return items;
    const q = query.toLowerCase();
    return items.filter((item) => item.label.toLowerCase().includes(q));
  }, [items, query]);

  return { open, query, setQuery, toggle, close, filtered };
}
