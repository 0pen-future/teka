import { useParams } from "react-router";

import { useNoIndex } from "@/lib/hooks/use-no-index";

import { StatementError } from "../components/statement-error";
import { StatementSkeleton } from "../components/statement-skeleton";
import { StatementView } from "../components/statement-view";
import { useStatement } from "../hooks/use-statement";

/**
 * Route element for `/s/:token` — a public, read-only parent statement. No
 * distinction is drawn between "loading forever because the token is
 * missing" and any other pending state: the router only ever mounts this
 * component with a `token` param present.
 */
export function StatementPage() {
  const { token } = useParams<{ token: string }>();
  useNoIndex();
  const { data, isPending, isError } = useStatement(token);

  if (isError || !token) {
    return <StatementError />;
  }
  if (isPending) {
    return <StatementSkeleton />;
  }
  return <StatementView statement={data} />;
}
