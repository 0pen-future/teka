// Public surface of the profile feature. Other features import ONLY from
// here; routes.tsx stays a separate entry so the router can mount pages
// without pulling them into every consumer's chunk.
export { ZALO_MATCH_MAX_PHONES, ZALO_MATCH_REQUEST_SIZE } from "./api/zalo-api";
export {
  useMatchZaloFriends,
  useSendZaloFriendRequest,
  useZaloFriends,
  useZaloStatus,
} from "./hooks/use-zalo";
export type { ZaloFriend, ZaloFriendMatch } from "./schemas/zalo-schemas";
