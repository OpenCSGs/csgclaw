import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ModelMetadataFields } from "./ModelMetadataFields";
import type { TranslateFn } from "@/models/conversations";
const t: TranslateFn = (key) => key;
describe("model context row", () => {
  it("edits M tokens as tokens and gives immediate feedback", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <ModelMetadataFields
        model="luna"
        automatic={{ context_window: 1050000, context_source: "catalog" }}
        onChange={onChange}
        onRefresh={vi.fn().mockResolvedValue(undefined)}
        t={t}
      />,
    );
    expect(screen.getByText("1.05 M tokens")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "luna modelMetadataContext" }));
    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "2");
    await user.click(screen.getByRole("button", { name: "modelContextApply" }));
    expect(onChange).toHaveBeenCalledWith({ context_window: 2000000 });
    expect(screen.getByRole("status")).toHaveTextContent("modelContextUpdated");
  });
  it("keeps invalid edits open and resets to the automatic value", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    const view = render(
      <ModelMetadataFields
        model="m"
        automatic={{ context_window: 1000000, context_source: "catalog" }}
        value={{ context_window: 8192 }}
        onChange={onChange}
        onRefresh={vi.fn().mockResolvedValue(undefined)}
        t={t}
      />,
    );
    await user.click(screen.getByRole("button", { name: "m modelMetadataContext" }));
    await user.clear(screen.getByRole("textbox"));
    await user.type(screen.getByRole("textbox"), "-1{Enter}");
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("modelContextInvalid");
    await user.click(screen.getByRole("button", { name: "modelMetadataReset" }));
    expect(onChange).toHaveBeenCalledWith(undefined);
    view.rerender(
      <ModelMetadataFields
        model="m"
        automatic={{ context_window: 1000000, context_source: "catalog" }}
        onChange={onChange}
        onRefresh={vi.fn().mockResolvedValue(undefined)}
        changed
        t={t}
      />,
    );
    expect(screen.getByText("1 M tokens")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("modelContextResetDone");
  });
});

it("refreshes automatic metadata with progress and preserves manual overrides", async () => {
  let complete!: () => void;
  const onRefresh = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        complete = resolve;
      }),
  );
  const onChange = vi.fn();
  const user = userEvent.setup();
  render(
    <ModelMetadataFields
      model="m"
      automatic={{ context_window: 1000000, context_source: "catalog" }}
      value={{ context_window: 200000 }}
      onChange={onChange}
      onRefresh={onRefresh}
      t={t}
    />,
  );
  const refresh = screen.getByRole("button", { name: "m modelContextRefresh" });
  await user.click(refresh);
  expect(onRefresh).toHaveBeenCalledTimes(1);
  expect(refresh).toHaveAttribute("aria-busy", "true");
  expect(screen.getByRole("status")).toHaveTextContent("modelContextRefreshing");
  await act(async () => complete());
  expect(screen.getByRole("status")).toHaveTextContent("modelContextRefreshed");
  expect(onChange).not.toHaveBeenCalled();
  expect(screen.getByText("200 K tokens")).toBeInTheDocument();
});
it("allows refresh without overrides and reports a failure", async () => {
  const user = userEvent.setup();
  render(
    <ModelMetadataFields
      model="m"
      onChange={vi.fn()}
      onRefresh={vi.fn().mockRejectedValue(new Error("offline"))}
      t={t}
    />,
  );
  await user.click(screen.getByRole("button", { name: "m modelContextRefresh" }));
  expect(screen.getByRole("status")).toHaveTextContent("modelContextRefreshFailed");
});
