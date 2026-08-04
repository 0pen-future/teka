// Public surface of the statement feature. The app router imports ONLY from
// here to mount the public route; nothing else in the app needs to depend on
// this feature.
export { STATEMENT_PATH_PREFIX, statementRoutes } from "./routes";
