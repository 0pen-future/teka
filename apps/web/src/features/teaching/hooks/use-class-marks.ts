import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import { getMonthMarks } from "../api/teaching-api";
import { personalNoteKey, type TeachingState } from "../lib/teaching-store";
import { teachingKeys } from "./teaching-keys";

export interface ClassMarks {
  sessionNotes: TeachingState["sessionNotes"];
  sessionScores: TeachingState["sessionScores"];
  personalNotes: TeachingState["personalNotes"];
  pending: boolean;
}

const EMPTY_MARKS: Omit<ClassMarks, "pending"> = {
  sessionNotes: {},
  sessionScores: {},
  personalNotes: {},
};

/**
 * The month's session notes, scores, and per-student personal notes for a
 * class, reassembled from the batch read into the record-map slices the
 * classbook/records components consume (`TeachingState`). `month` is
 * `YYYY-MM`.
 */
export function useClassMarks(classId: string | undefined, month: string): ClassMarks {
  const query = useQuery({
    queryKey: teachingKeys.marks(classId ?? "", month),
    queryFn: () => getMonthMarks(classId!, month),
    enabled: Boolean(classId),
  });

  const slices = useMemo(() => {
    const data = query.data;
    if (!data) {
      return EMPTY_MARKS;
    }
    const sessionNotes: TeachingState["sessionNotes"] = {};
    for (const note of data.session_notes) {
      sessionNotes[note.session_id] = { text: note.body };
    }
    const sessionScores: TeachingState["sessionScores"] = {};
    const personalNotes: TeachingState["personalNotes"] = {};
    for (const mark of data.marks) {
      if (mark.score !== null) {
        (sessionScores[mark.session_id] ??= {})[mark.student_id] = mark.score;
      }
      if (mark.personal_note !== null) {
        personalNotes[personalNoteKey(mark.session_id, mark.student_id)] = mark.personal_note;
      }
    }
    return { sessionNotes, sessionScores, personalNotes };
  }, [query.data]);

  return { ...slices, pending: query.isPending };
}
