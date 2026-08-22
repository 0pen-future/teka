// Public surface of the invitation feature. Other features (the app router,
// the center feature's owner card) import ONLY from here; routes.tsx stays a
// separate entry so the router can mount the public accept page without
// pulling it into every consumer's chunk.
export { InviteSection } from "./components/invite-section";
export { invitationRoutes } from "./routes";
