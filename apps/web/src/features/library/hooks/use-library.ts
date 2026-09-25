import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";

import {
  archiveVersion,
  clearLessons,
  createExercise,
  createExerciseGroup,
  createLesson,
  createMaterial,
  createTemplate,
  createVersion,
  deleteExercise,
  deleteExerciseGroup,
  deleteLesson,
  deleteMaterial,
  deleteTemplate,
  duplicateLesson,
  getLesson,
  getTemplate,
  getVersion,
  listExerciseGroups,
  listExercises,
  listLessons,
  listMaterials,
  listTemplates,
  listVersions,
  publishVersion,
  reorderLessons,
  setExerciseStatus,
  setLessonExercises,
  setLessonMaterials,
  setLogFields,
  setMaterialStatus,
  setScoreSet,
  updateExercise,
  updateLesson,
  updateMaterial,
  updateTemplate,
  type ListItemsParams,
  type ListTemplatesParams,
} from "../api/library-api";
import type {
  ExerciseGroupInput,
  ExerciseInput,
  LessonExerciseInput,
  LessonInput,
  LessonMaterialInput,
  LogFieldInput,
  MaterialInput,
  ScoreSetGroupInput,
  TemplateInput,
} from "../schemas/library-schemas";
import {
  exerciseGroupsKeys,
  exercisesKeys,
  lessonsKeys,
  materialsKeys,
  templatesKeys,
  versionsKeys,
} from "./library-keys";

export {
  exerciseGroupsKeys,
  exercisesKeys,
  lessonsKeys,
  materialsKeys,
  templatesKeys,
  versionsKeys,
};

export function useTemplatesList(params: ListTemplatesParams = {}, enabled = true) {
  return useQuery({
    queryKey: templatesKeys.list(params),
    queryFn: () => listTemplates(params),
    enabled,
    placeholderData: keepPreviousData,
  });
}

export function useTemplate(id: string | undefined) {
  return useQuery({
    queryKey: templatesKeys.detail(id ?? ""),
    queryFn: () => getTemplate(id ?? ""),
    enabled: Boolean(id),
  });
}

export function useCreateTemplate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: TemplateInput) => createTemplate(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: templatesKeys.lists() });
    },
  });
}

export function useUpdateTemplate(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: TemplateInput) => updateTemplate(id, input),
    onSuccess: (template) => {
      queryClient.setQueryData(templatesKeys.detail(id), template);
      void queryClient.invalidateQueries({ queryKey: templatesKeys.lists() });
    },
  });
}

export function useDeleteTemplate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteTemplate(id),
    onSuccess: (_data, id) => {
      queryClient.removeQueries({ queryKey: templatesKeys.detail(id) });
      void queryClient.invalidateQueries({ queryKey: templatesKeys.lists() });
    },
  });
}

export function useVersions(templateId: string | undefined) {
  return useQuery({
    queryKey: versionsKeys.list(templateId ?? ""),
    queryFn: () => listVersions(templateId ?? ""),
    enabled: Boolean(templateId),
  });
}

/** Refetch the template too: its draft/published summary and version count moved. */
function useVersionWrite<TVars, TData>(
  templateId: string,
  mutationFn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    // Settled, not success: a 409 means the version changed under the
    // caller, and the refetch is what replaces the stale row and its buttons.
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: versionsKeys.list(templateId) });
      void queryClient.invalidateQueries({ queryKey: templatesKeys.detail(templateId) });
      void queryClient.invalidateQueries({ queryKey: templatesKeys.lists() });
    },
  });
}

export function useCreateVersion(templateId: string) {
  return useVersionWrite(templateId, (changelog: string | null) =>
    createVersion(templateId, changelog),
  );
}

export function usePublishVersion(templateId: string) {
  return useVersionWrite(templateId, (versionId: string) => publishVersion(versionId));
}

export function useArchiveVersion(templateId: string) {
  return useVersionWrite(templateId, (versionId: string) => archiveVersion(versionId));
}

export function useLessons(versionId: string | undefined) {
  return useQuery({
    queryKey: lessonsKeys.list(versionId ?? ""),
    queryFn: () => listLessons(versionId ?? ""),
    enabled: Boolean(versionId),
  });
}

export function useLesson(id: string | undefined) {
  return useQuery({
    queryKey: lessonsKeys.detail(id ?? ""),
    queryFn: () => getLesson(id ?? ""),
    enabled: Boolean(id),
  });
}

/**
 * Invalidates every cache a version's lesson or exercise-group content
 * changes stale: the lessons list and every cached lesson detail (a group
 * delete clears the `group_id` those details embed), the version's own
 * detail (its aggregate exercises/materials tabs) and its exercise groups,
 * and — because lesson and group counts roll up into the template's card
 * counts and the material/exercise banks' "Dùng trong" column — the
 * template lists/detail and the catalog bank lists too.
 */
export function invalidateVersionContent(
  queryClient: QueryClient,
  versionId: string,
  templateId?: string,
) {
  void queryClient.invalidateQueries({ queryKey: lessonsKeys.list(versionId) });
  void queryClient.invalidateQueries({ queryKey: lessonsKeys.details() });
  void queryClient.invalidateQueries({ queryKey: versionsKeys.detail(versionId) });
  void queryClient.invalidateQueries({ queryKey: exerciseGroupsKeys.list(versionId) });
  void queryClient.invalidateQueries({ queryKey: templatesKeys.lists() });
  void queryClient.invalidateQueries({ queryKey: materialsKeys.lists() });
  void queryClient.invalidateQueries({ queryKey: exercisesKeys.lists() });
  if (templateId) {
    void queryClient.invalidateQueries({ queryKey: versionsKeys.list(templateId) });
    void queryClient.invalidateQueries({ queryKey: templatesKeys.detail(templateId) });
  }
}

/**
 * Lesson writes change the version's lesson count, so the version list
 * refetches alongside the lessons. Reorder and delete also renumber the
 * sibling lessons, so every cached lesson detail goes stale with them.
 */
function useLessonListWrite<TVars, TData>(
  versionId: string,
  templateId: string,
  mutationFn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSettled: () => {
      invalidateVersionContent(queryClient, versionId, templateId);
    },
  });
}

export function useCreateLesson(versionId: string, templateId: string) {
  return useLessonListWrite(versionId, templateId, (input: LessonInput) =>
    createLesson(versionId, input),
  );
}

export function useDeleteLesson(versionId: string, templateId: string) {
  return useLessonListWrite(versionId, templateId, (id: string) => deleteLesson(id));
}

export function useReorderLessons(versionId: string, templateId: string) {
  return useLessonListWrite(versionId, templateId, (lessonIds: string[]) =>
    reorderLessons(versionId, lessonIds),
  );
}

export function useUpdateLesson(id: string, versionId: string, templateId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: LessonInput) => updateLesson(id, input),
    onSuccess: (lesson) => {
      queryClient.setQueryData(lessonsKeys.detail(id), lesson);
    },
    // Settled, not success: a 409 VERSION_LOCKED means the version was
    // published under the author, and refetching the versions is what
    // swaps the editor for the locked read-only view.
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: lessonsKeys.list(versionId) });
      void queryClient.invalidateQueries({ queryKey: lessonsKeys.detail(id) });
      void queryClient.invalidateQueries({ queryKey: versionsKeys.list(templateId) });
    },
  });
}

export function useDuplicateLesson(versionId: string, templateId: string) {
  return useLessonListWrite(versionId, templateId, (id: string) => duplicateLesson(id));
}

export function useClearLessons(versionId: string, templateId: string) {
  return useLessonListWrite(versionId, templateId, () => clearLessons(versionId));
}

/**
 * A version's own content: score set, log fields and lesson details. Read
 * separately from the version list because it is only needed by the
 * grading panel of one version at a time.
 */
export function useVersionDetail(versionId: string | undefined) {
  return useQuery({
    queryKey: versionsKeys.detail(versionId ?? ""),
    queryFn: () => getVersion(versionId ?? ""),
    enabled: Boolean(versionId),
  });
}

/**
 * Wholesale replace of one version-level list. Settled, not success: a
 * 409 VERSION_LOCKED means the version was published under the author, and
 * refetching the versions is what swaps the editor for the read-only view.
 */
function useVersionContentWrite<TVars, TData>(
  versionId: string,
  templateId: string,
  mutationFn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: versionsKeys.detail(versionId) });
      void queryClient.invalidateQueries({ queryKey: versionsKeys.list(templateId) });
    },
  });
}

export function useSetLogFields(versionId: string, templateId: string) {
  return useVersionContentWrite(versionId, templateId, (items: LogFieldInput[]) =>
    setLogFields(versionId, items),
  );
}

export function useSetScoreSet(versionId: string, templateId: string) {
  return useVersionContentWrite(versionId, templateId, (items: ScoreSetGroupInput[]) =>
    setScoreSet(versionId, items),
  );
}

export function useExerciseGroups(versionId: string | undefined) {
  return useQuery({
    queryKey: exerciseGroupsKeys.list(versionId ?? ""),
    queryFn: () => listExerciseGroups(versionId ?? ""),
    enabled: Boolean(versionId),
  });
}

/** Exercise-group writes also change the version's own lesson-exercise `group_id` links. */
function useExerciseGroupWrite<TVars, TData>(
  versionId: string,
  templateId: string,
  mutationFn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSettled: () => {
      invalidateVersionContent(queryClient, versionId, templateId);
    },
  });
}

export function useCreateExerciseGroup(versionId: string, templateId: string) {
  return useExerciseGroupWrite(versionId, templateId, (input: ExerciseGroupInput) =>
    createExerciseGroup(versionId, input),
  );
}

export function useDeleteExerciseGroup(versionId: string, templateId: string) {
  return useExerciseGroupWrite(versionId, templateId, (groupId: string) =>
    deleteExerciseGroup(versionId, groupId),
  );
}

/**
 * Attachment writes change the lesson detail; `invalidateVersionContent`
 * already covers it via `lessonsKeys.details()`, plus the lessons list, the
 * version's aggregate tabs and the material/exercise banks' "Dùng trong"
 * column, which all roll up the same attached counts.
 */
function useLessonAttachmentWrite<TVars, TData>(
  versionId: string,
  templateId: string,
  mutationFn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSettled: () => {
      invalidateVersionContent(queryClient, versionId, templateId);
    },
  });
}

export function useSetLessonMaterials(lessonId: string, versionId: string, templateId: string) {
  return useLessonAttachmentWrite(versionId, templateId, (items: LessonMaterialInput[]) =>
    setLessonMaterials(lessonId, items),
  );
}

export function useSetLessonExercises(lessonId: string, versionId: string, templateId: string) {
  return useLessonAttachmentWrite(versionId, templateId, (items: LessonExerciseInput[]) =>
    setLessonExercises(lessonId, items),
  );
}

export function useMaterialsList(params: ListItemsParams = {}, enabled = true) {
  return useQuery({
    queryKey: materialsKeys.list(params),
    queryFn: () => listMaterials(params),
    enabled,
    placeholderData: keepPreviousData,
  });
}

export function useExercisesList(params: ListItemsParams = {}, enabled = true) {
  return useQuery({
    queryKey: exercisesKeys.list(params),
    queryFn: () => listExercises(params),
    enabled,
    placeholderData: keepPreviousData,
  });
}

/**
 * Catalog writes: the list refetches, and because lessons embed the
 * catalog row (title, kind, tags) every cached lesson and version detail
 * goes stale too. Delete cannot be undone by a refetch, so 409 IN_USE is
 * left to the caller to surface.
 */
function useCatalogWrite<TVars, TData>(
  listsKey: readonly unknown[],
  mutationFn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    // onSettled: a failed write (a 409 on delete, a lost response) still
    // means the cache may be behind the server.
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: listsKey });
      void queryClient.invalidateQueries({ queryKey: lessonsKeys.details() });
      void queryClient.invalidateQueries({ queryKey: versionsKeys.details() });
    },
  });
}

export function useCreateMaterial() {
  return useCatalogWrite(materialsKeys.lists(), (input: MaterialInput) => createMaterial(input));
}

export function useUpdateMaterial(id: string) {
  return useCatalogWrite(materialsKeys.lists(), (input: MaterialInput) =>
    updateMaterial(id, input),
  );
}

export function useDeleteMaterial() {
  return useCatalogWrite(materialsKeys.lists(), (id: string) => deleteMaterial(id));
}

export function useSetMaterialStatus() {
  return useCatalogWrite(materialsKeys.lists(), (vars: { id: string; active: boolean }) =>
    setMaterialStatus(vars.id, vars.active),
  );
}

export function useCreateExercise() {
  return useCatalogWrite(exercisesKeys.lists(), (input: ExerciseInput) => createExercise(input));
}

export function useUpdateExercise(id: string) {
  return useCatalogWrite(exercisesKeys.lists(), (input: ExerciseInput) =>
    updateExercise(id, input),
  );
}

export function useDeleteExercise() {
  return useCatalogWrite(exercisesKeys.lists(), (id: string) => deleteExercise(id));
}

export function useSetExerciseStatus() {
  return useCatalogWrite(exercisesKeys.lists(), (vars: { id: string; active: boolean }) =>
    setExerciseStatus(vars.id, vars.active),
  );
}
