import { Suspense } from "react";
import { Outlet } from "react-router";

import { RouteDocumentTitle } from "@/components/shared/document-title";
import { ErrorBoundary } from "@/components/shared/error-boundary";
import { Spinner } from "@/components/shared/spinner";

/** Outermost shell: tab title, skip link, error boundary, and suspense fallback. */
export function RootLayout() {
  return (
    <>
      <RouteDocumentTitle />
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:shadow"
      >
        Bỏ qua đến nội dung chính
      </a>
      <ErrorBoundary>
        <Suspense
          fallback={
            <div className="flex min-h-svh items-center justify-center">
              <Spinner />
            </div>
          }
        >
          <Outlet />
        </Suspense>
      </ErrorBoundary>
    </>
  );
}
