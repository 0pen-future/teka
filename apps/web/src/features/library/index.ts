// Public surface of the library feature. Routes stay in routes.tsx so the
// router mounts pages without pulling them into every consumer's chunk.
export {
  lessonsKeys,
  templatesKeys,
  useLessons,
  useTemplate,
  useTemplatesList,
  useVersions,
  versionsKeys,
} from "./hooks/use-library";
export { templateLessonDetailSchema } from "./schemas/library-schemas";
export type {
  LessonExercise,
  LessonMaterial,
  ProgramTemplate,
  TemplateLesson,
  TemplateLessonDetail,
  TemplateVersion,
  TemplateVersionStatus,
} from "./schemas/library-schemas";
export {
  defaultVersion,
  formatDifficulty,
  materialKindLabel,
  versionLabel,
  versionStatusLabel,
} from "./lib/library-labels";
export { rowErrorsFromApi } from "./lib/row-errors";
export type { RowErrors } from "./lib/row-errors";
