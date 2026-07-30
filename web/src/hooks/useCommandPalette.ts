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
      { id: "nav-apps", label: "Apps", section: "Navigate", icon: "A", action: () => navigate("/apps") },
      { id: "nav-code", label: "Code", section: "Navigate", icon: "<", action: () => navigate("/code") },
      { id: "nav-templates", label: "Marketplace", section: "Navigate", icon: "T", action: () => navigate("/templates") },
      // Host tools — the sidebar shows the host machine's name; the
      // palette keeps a static label since it doesn't fetch system info.
      { id: "nav-tools", label: "Host", section: "Navigate", icon: "t", action: () => navigate("/tools") },
      { id: "nav-custom-keys", label: "Custom Keys", section: "Navigate", icon: "#", action: () => navigate("/apps#custom-keys") },
      { id: "nav-metrics", label: "Insights: Metrics", section: "Navigate", icon: "M", action: () => navigate("/insights?tab=metrics") },
      { id: "nav-costs", label: "Insights: Costs", section: "Navigate", icon: "$", action: () => navigate("/insights?tab=costs") },
      { id: "nav-settings", label: "Settings", section: "Navigate", icon: "\u2699", action: () => navigate("/settings") },
      // Actions
      { id: "act-create-agent", label: "Create Agent", section: "Action", icon: "+", action: () => navigate("/agents?action=create") },
      { id: "act-connect-app", label: "Connect App", section: "Action", icon: "+", action: () => navigate("/apps?action=connect") },
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
