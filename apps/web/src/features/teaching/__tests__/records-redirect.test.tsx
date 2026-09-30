import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { describe, expect, it } from "vitest";

import { RecordsRedirect, StudentRecordRedirect } from "../components/records-redirect";

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>;
}

function renderAt(url: string) {
  render(
    <MemoryRouter initialEntries={[url]}>
      <Routes>
        <Route path="/records" element={<RecordsRedirect />} />
        <Route path="/records/:studentId" element={<StudentRecordRedirect />} />
        <Route path="/students" element={<LocationProbe />} />
        <Route path="/students/:id" element={<LocationProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("records redirects", () => {
  it("sends /records to the students page, keeping only class_id and q", () => {
    renderAt("/records?class_id=c1&q=an&sort=x");
    expect(screen.getByTestId("location").textContent).toBe("/students?class_id=c1&q=an");
  });

  it("sends /records without params to the bare students page", () => {
    renderAt("/records");
    expect(screen.getByTestId("location").textContent).toBe("/students");
  });

  it("sends /records/:studentId to that student's detail page", () => {
    renderAt("/records/s1");
    expect(screen.getByTestId("location").textContent).toBe("/students/s1");
  });
});
