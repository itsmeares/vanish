import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./app.css";
import { App } from "./app";

// The app follows the system's light or dark setting.
const dark = matchMedia("(prefers-color-scheme: dark)");
const applyTheme = () =>
  document.documentElement.classList.toggle("dark", dark.matches);
applyTheme();
dark.addEventListener("change", applyTheme);

const root = document.getElementById("root");
if (root)
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
