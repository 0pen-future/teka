// Public surface of the collections feature. Other features import ONLY
// from here; `routes.tsx` stays a separate entry so the router can mount
// pages without pulling them into every consumer's chunk.
export {
  collectionsKeys,
  useClassCollectionsList,
  useCollectionsSummary,
  useContactCollectionsList,
  usePeriod,
  useReallocatePayment,
  useRecordPayment,
} from "./hooks/use-collections";
export {
  notificationsKeys,
  useBulkSendNotifications,
  useMarkNotificationsSent,
  useNotificationsList,
} from "./hooks/use-notifications";

export {
  allocatedBySchema,
  allocationResponseSchema,
  bulkSendInputSchema,
  bulkSendResponseSchema,
  bulkSendRowSchema,
  classCollectionRowSchema,
  collectionsSummarySchema,
  contactBalanceRowSchema,
  contactChildInvoiceRowSchema,
  notificationChannelSchema,
  notificationPurposeSchema,
  notificationRowSchema,
  notificationStatusSchema,
  paymentMethodSchema,
  paymentResponseSchema,
  paymentStatusSchema,
  reallocateInputSchema,
  recordPaymentInputSchema,
} from "./schemas/collections-schemas";
export type {
  AllocatedBy,
  AllocationResponse,
  BulkSendInput,
  BulkSendResponse,
  BulkSendRow,
  ClassCollectionRow,
  CollectionsSummary,
  ContactBalanceRow,
  ContactChildInvoiceRow,
  NotificationChannel,
  NotificationPurpose,
  NotificationRow,
  NotificationStatus,
  PaymentMethod,
  PaymentResponse,
  PaymentStatus,
  ReallocateInput,
  RecordPaymentInput,
} from "./schemas/collections-schemas";

export type {
  CollectionsStatusFilter,
  CollectionsView,
  ListClassCollectionsParams,
  ListContactCollectionsParams,
} from "./types/collections-types";
export { COLLECTIONS_VIEWS } from "./types/collections-types";
