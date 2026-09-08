import { screen } from "@testing-library/react";
import type userEvent from "@testing-library/user-event";

/**
 * Picks an `HvSelect` option: clicks the combobox trigger, then the option by
 * name. Options render in a Radix portal outside any dialog, so they are
 * always looked up through the global `screen`, never `within(dialog)` —
 * only the trigger lookup takes a scoped container.
 */
export async function pickOption(
  user: ReturnType<typeof userEvent.setup>,
  container: { getByRole: typeof screen.getByRole },
  comboboxName: string | RegExp,
  optionName: string | RegExp,
): Promise<void> {
  const combobox = container.getByRole("combobox", { name: comboboxName });
  await user.click(combobox);
  const option = await screen.findByRole("option", { name: optionName });
  await user.click(option);
}
