// Public surface of the course catalog. Routes stay in routes.tsx so the
// router mounts pages without pulling them into every consumer's chunk.
export { coursesKeys } from "./hooks/courses-keys";
export { useCourse, useCoursesList } from "./hooks/use-courses";
export type { Course, CourseStatus, TuitionPack } from "./schemas/courses-schemas";
export { courseStatusLabel, courseStatusVariant } from "./lib/course-labels";
