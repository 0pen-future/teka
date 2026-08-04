/**
 * Query key factories for every roster entity, kept in one module so the
 * four `use-*` hook files can invalidate across entity boundaries (see the
 * cache invalidation graph in the phase's Architecture section) without
 * importing from one another.
 */
import type { ListClassesParams } from "../api/classes-api";
import type { ListContactsParams } from "../api/contacts-api";
import type { ListEnrollmentsParams } from "../api/enrollments-api";
import type { ListStudentsParams } from "../api/students-api";

export const contactsKeys = {
  all: ["roster", "contacts"] as const,
  lists: () => [...contactsKeys.all, "list"] as const,
  list: (params: ListContactsParams) => [...contactsKeys.lists(), params] as const,
  details: () => [...contactsKeys.all, "detail"] as const,
  detail: (id: string) => [...contactsKeys.details(), id] as const,
};

export const studentsKeys = {
  all: ["roster", "students"] as const,
  lists: () => [...studentsKeys.all, "list"] as const,
  list: (params: ListStudentsParams) => [...studentsKeys.lists(), params] as const,
  details: () => [...studentsKeys.all, "detail"] as const,
  detail: (id: string) => [...studentsKeys.details(), id] as const,
};

export const classesKeys = {
  all: ["roster", "classes"] as const,
  lists: () => [...classesKeys.all, "list"] as const,
  list: (params: ListClassesParams) => [...classesKeys.lists(), params] as const,
  details: () => [...classesKeys.all, "detail"] as const,
  detail: (id: string) => [...classesKeys.details(), id] as const,
};

export const enrollmentsKeys = {
  all: ["roster", "enrollments"] as const,
  lists: () => [...enrollmentsKeys.all, "list"] as const,
  list: (params: ListEnrollmentsParams) => [...enrollmentsKeys.lists(), params] as const,
  details: () => [...enrollmentsKeys.all, "detail"] as const,
  detail: (id: string) => [...enrollmentsKeys.details(), id] as const,
};
