import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  getCollectionsSummary,
  getPeriod,
  listClassCollections,
  listContactCollections,
  recordPayment,
  reallocatePayment,
} from "../api/collections-api";
import type { ReallocateInput, RecordPaymentInput } from "../schemas/collections-schemas";
import type {
  ListClassCollectionsParams,
  ListContactCollectionsParams,
} from "../types/collections-types";

/**
 * Query key factory for the collections domain (period header, both
 * collection views, and the summary bar). Kept in one place so payment
 * mutations can invalidate every dependent query without importing across
 * hook files.
 */
export const collectionsKeys = {
  all: ["collections"] as const,
  period: (periodId: string) => [...collectionsKeys.all, "period", periodId] as const,
  lists: () => [...collectionsKeys.all, "list"] as const,
  contactList: (periodId: string, params: ListContactCollectionsParams) =>
    [...collectionsKeys.lists(), "contact", periodId, params] as const,
  classList: (periodId: string, params: ListClassCollectionsParams) =>
    [...collectionsKeys.lists(), "class", periodId, params] as const,
  summaries: () => [...collectionsKeys.all, "summary"] as const,
  summary: (periodId: string) => [...collectionsKeys.summaries(), periodId] as const,
};

export function usePeriod(periodId: string | undefined) {
  return useQuery({
    queryKey: collectionsKeys.period(periodId ?? ""),
    queryFn: () => getPeriod(periodId!),
    enabled: Boolean(periodId),
  });
}

export function useContactCollectionsList(
  periodId: string | undefined,
  params: ListContactCollectionsParams = {},
) {
  return useQuery({
    queryKey: collectionsKeys.contactList(periodId ?? "", params),
    queryFn: () => listContactCollections(periodId!, params),
    enabled: Boolean(periodId),
    placeholderData: keepPreviousData,
  });
}

export function useClassCollectionsList(
  periodId: string | undefined,
  params: Omit<ListClassCollectionsParams, "class_id"> & { class_id?: string },
) {
  const classId = params.class_id;
  return useQuery({
    queryKey: collectionsKeys.classList(periodId ?? "", { ...params, class_id: classId ?? "" }),
    queryFn: () => listClassCollections(periodId!, { ...params, class_id: classId! }),
    enabled: Boolean(periodId) && Boolean(classId),
    placeholderData: keepPreviousData,
  });
}

export function useCollectionsSummary(periodId: string | undefined) {
  return useQuery({
    queryKey: collectionsKeys.summary(periodId ?? ""),
    queryFn: () => getCollectionsSummary(periodId!),
    enabled: Boolean(periodId),
  });
}

/**
 * Records a payment. The response already carries the server's auto-
 * allocated split (D8 rule) — there is no separate preview endpoint, so
 * `RecordPaymentDialog` shows this same response as the "preview" the
 * teacher can then correct via `useReallocatePayment`.
 */
export function useRecordPayment(periodId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: RecordPaymentInput) => recordPayment(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: collectionsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: collectionsKeys.summary(periodId) });
    },
  });
}

export function useReallocatePayment(periodId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ paymentId, input }: { paymentId: string; input: ReallocateInput }) =>
      reallocatePayment(paymentId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: collectionsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: collectionsKeys.summary(periodId) });
    },
  });
}
