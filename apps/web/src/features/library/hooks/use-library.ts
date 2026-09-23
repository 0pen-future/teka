import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  archiveVersion,
  createLesson,
  createTemplate,
  createVersion,
  deleteLesson,
  deleteTemplate,
  getLesson,
  getTemplate,
  listLessons,
  listTemplates,
  listVersions,
  publishVersion,
  reorderLessons,
  updateLesson,
  updateTemplate,
  type ListTemplatesParams,
} from "../api/library-api";
import type { LessonInput, TemplateInput } from "../schemas/library-schemas";
import { lessonsKeys, templatesKeys, versionsKeys } from "./library-keys";

export { lessonsKeys, templatesKeys, versionsKeys };

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
      void queryClient.invalidateQueries({ queryKey: lessonsKeys.list(versionId) });
      void queryClient.invalidateQueries({ queryKey: lessonsKeys.details() });
      void queryClient.invalidateQueries({ queryKey: versionsKeys.list(templateId) });
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
