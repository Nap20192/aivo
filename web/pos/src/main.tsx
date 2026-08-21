import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "../../design-system/styles.css";
import "./pos.css";
import App from "./App.tsx";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);

if ("serviceWorker" in navigator && !import.meta.env.DEV) {
  navigator.serviceWorker.register(import.meta.env.BASE_URL + "sw.js").catch(() => {});
}
