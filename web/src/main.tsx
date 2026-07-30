import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";

// Geist Sans + Geist Mono via @fontsource. We import only the weights the
// dashboard actually uses (400 normal, 500 medium, 600 semibold for sans;
// 400 + 600 for mono) to keep the initial bundle slim and avoid a FOIT.
import "@fontsource/geist-sans/400.css";
import "@fontsource/geist-sans/500.css";
import "@fontsource/geist-sans/600.css";
import "@fontsource/geist-mono/400.css";
import "@fontsource/geist-mono/600.css";

// Fraunces — the landing's display serif, used ONLY as a rare accent
// (.font-display): drawer wordmark, page titles, empty-state headings.
import "@fontsource-variable/fraunces";

import "./theme/tokens.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
