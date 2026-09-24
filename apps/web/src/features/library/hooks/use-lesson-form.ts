import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, type UseFormReturn } from "react-hook-form";

import {
  lessonFormSchema,
  type LessonFormInput,
  type LessonFormValues,
} from "../schemas/library-schemas";

export const EMPTY_LESSON_FORM: LessonFormInput = {
  title: "",
  mode: "scheduled",
  unit: "",
  objectives: "",
  duration_min: "",
  homework_note: "",
};

export type LessonForm = UseFormReturn<LessonFormInput, unknown, LessonFormValues>;

/** One resolver for the add dialog and the lesson page so both validate identically. */
export function useLessonForm(defaultValues: LessonFormInput = EMPTY_LESSON_FORM): LessonForm {
  return useForm<LessonFormInput, unknown, LessonFormValues>({
    resolver: zodResolver(lessonFormSchema),
    defaultValues,
  });
}
