// Public surface of the attendance feature. Other features import ONLY from
// here; routes.tsx stays a separate entry so the router can mount pages
// without pulling them into every consumer's chunk.
export { useSession, useSessionRoster, useSessionsList, sessionsKeys } from "./hooks/use-sessions";

// Raw fetcher for consumers that need a dynamic number of per-class queries
// (`useQueries`), which a fixed hook call cannot express.
export { listClassSessions } from "./api/attendance-api";

export {
  sessionSchema,
  attendanceRowSchema,
  attendanceResponseSchema,
} from "./schemas/attendance-schemas";
export type { Session, AttendanceRow, AttendanceResponse } from "./schemas/attendance-schemas";
