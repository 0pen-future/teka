// Public surface of the profile feature. Other features import ONLY from
// here; routes.tsx stays a separate entry so the router can mount pages
// without pulling them into every consumer's chunk.
export { useZaloFriends, useZaloStatus } from "./hooks/use-zalo";
export type { ZaloFriend } from "./schemas/zalo-schemas";
export { getZaloFriends } from "./api/zalo-api";
