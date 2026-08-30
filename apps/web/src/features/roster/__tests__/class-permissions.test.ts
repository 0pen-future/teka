import { describe, expect, it } from "vitest";

import { canRecordAttendance, canSendClassReports, canWriteClass } from "../lib/class-permissions";

const withRoles = (roles: string[]) => ({ my_staff_roles: roles });

describe("canWriteClass", () => {
  it("opens for the owner and the giao_vien staff only", () => {
    expect(canWriteClass(true, withRoles([]))).toBe(true);
    expect(canWriteClass(false, withRoles(["giao_vien"]))).toBe(true);
    expect(canWriteClass(false, withRoles(["tro_giang", "hoc_vu"]))).toBe(false);
    expect(canWriteClass(false, withRoles([]))).toBe(false);
  });
});

describe("canRecordAttendance", () => {
  it("mirrors the API attendance.write capability: giao_vien and tro_giang", () => {
    expect(canRecordAttendance(false, withRoles(["giao_vien"]))).toBe(true);
    expect(canRecordAttendance(false, withRoles(["tro_giang"]))).toBe(true);
    expect(canRecordAttendance(true, withRoles([]))).toBe(true);
  });

  it("stays closed for hoc_vu and stintless members", () => {
    expect(canRecordAttendance(false, withRoles(["hoc_vu"]))).toBe(false);
    expect(canRecordAttendance(false, withRoles([]))).toBe(false);
  });
});

describe("canSendClassReports", () => {
  it("mirrors the API statement.send capability: hoc_vu, plus reports oversight", () => {
    expect(canSendClassReports(false, withRoles(["hoc_vu"]))).toBe(true);
    expect(canSendClassReports(true, withRoles([]))).toBe(true);
  });

  it("stays closed for giao_vien, tro_giang, and stintless members", () => {
    expect(canSendClassReports(false, withRoles(["giao_vien"]))).toBe(false);
    expect(canSendClassReports(false, withRoles(["tro_giang"]))).toBe(false);
    expect(canSendClassReports(false, withRoles([]))).toBe(false);
  });
});
