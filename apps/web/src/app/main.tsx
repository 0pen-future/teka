import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "@/styles/globals.css";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("Root element #root not found");
}

// The app modules are imported dynamically so a config failure (env.ts throws
// on a build missing VITE_API_URL) surfaces as a readable message instead of a
// blank page from a green build.
try {
  const [{ RouterProvider }, { Providers }, { router }] = await Promise.all([
    import("react-router/dom"),
    import("@/app/providers"),
    import("@/app/router"),
  ]);

  createRoot(rootElement).render(
    <StrictMode>
      <Providers>
        <RouterProvider router={router} />
      </Providers>
    </StrictMode>,
  );
} catch (error) {
  console.error(error);
  const message = error instanceof Error ? error.message : String(error);
  const fallback = document.createElement("pre");
  fallback.style.cssText =
    "padding: 2rem; font-family: ui-monospace, monospace; white-space: pre-wrap; color: #b91c1c;";
  fallback.textContent = `Teka failed to start.\n\n${message}`;
  rootElement.replaceChildren(fallback);
}
