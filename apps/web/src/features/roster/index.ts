// Public surface of the roster feature. Other features import ONLY from
// here (types and read hooks for classes/students/enrollments they need to
// reference); routes.tsx stays a separate entry so the router can mount
// pages without pulling them into every consumer's chunk.
export { useClass, useClassesList, classesKeys } from "./hooks/use-classes";
export { ClassSearchEmptyNote, ClassSearchInput } from "./components/class-search";
export { useClassSearch } from "./hooks/use-class-search";
export { useContact, useContactsList, contactsKeys } from "./hooks/use-contacts";
export { useEnrollment, useEnrollmentsList, enrollmentsKeys } from "./hooks/use-enrollments";
export { useStudent, useStudentsList, studentsKeys } from "./hooks/use-students";

// Raw fetcher for consumers that need a dynamic number of per-class queries
// (`useQueries`), which a fixed hook call cannot express.
export { listStudents } from "./api/students-api";

export {
  classSchema,
  contactSchema,
  enrollmentSchema,
  scheduleSchema,
  studentSchema,
} from "./schemas/roster-schemas";
export type { Class, Contact, Enrollment, Schedule, Student } from "./schemas/roster-schemas";

export { formatScheduleSummary, formatWeekday } from "./lib/roster-format";
export { activeSchedules } from "./lib/schedule-diff";
