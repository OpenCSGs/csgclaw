import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EnvKeyValueEditor } from "@/components/business/ProfileControls";
import type { EnvKeyValueRow } from "@/components/business/ProfileControls";

const labels: Record<string, string> = {
  profileEnvAdd: "Add environment variable",
  profileEnvKey: "Key",
  profileEnvRemove: "Remove environment variable",
  profileEnvValue: "Value",
};

function t(key: string): string {
  return labels[key] ?? key;
}

function renderEditor(initialRows: EnvKeyValueRow[] = []) {
  function Harness() {
    const [rows, setRows] = useState(initialRows);
    return <EnvKeyValueEditor rows={rows} onChange={setRows} t={t} />;
  }

  return {
    user: userEvent.setup(),
    ...render(<Harness />),
  };
}

describe("EnvKeyValueEditor", () => {
  it("starts with one editable blank row when no env values exist", () => {
    renderEditor();

    expect(screen.getByPlaceholderText("Key")).toHaveValue("");
    expect(screen.getByPlaceholderText("Value")).toHaveValue("");
    expect(screen.getByRole("button", { name: "Add environment variable" })).toBeInTheDocument();
  });

  it("keeps user input visible through the controlled onChange flow", async () => {
    const { user } = renderEditor();

    await user.type(screen.getByPlaceholderText("Key"), "CSGCLAW_MODEL");
    await user.type(screen.getByPlaceholderText("Value"), "qwen3");

    expect(screen.getByPlaceholderText("Key")).toHaveValue("CSGCLAW_MODEL");
    expect(screen.getByPlaceholderText("Value")).toHaveValue("qwen3");
  });

  it("adds rows and falls back to one blank row after the last row is removed", async () => {
    const { user } = renderEditor([{ key: "HTTP_PROXY", value: "http://127.0.0.1:7890" }]);

    await user.click(screen.getByRole("button", { name: "Add environment variable" }));
    expect(screen.getAllByPlaceholderText("Key")).toHaveLength(2);

    await user.click(screen.getAllByRole("button", { name: "Remove environment variable" })[0]);
    expect(screen.getAllByPlaceholderText("Key")).toHaveLength(1);
    expect(screen.getByPlaceholderText("Key")).toHaveValue("");

    await user.click(screen.getByRole("button", { name: "Remove environment variable" }));
    expect(screen.getByPlaceholderText("Key")).toHaveValue("");
    expect(screen.getByPlaceholderText("Value")).toHaveValue("");
  });

  it("marks required template rows and keeps them present", async () => {
    const { container, user } = renderEditor([{ key: "GITLAB_TOKEN", required: true, value: "" }]);

    expect(container.querySelector(".env-required-star")).toHaveTextContent("*");
    expect(screen.getByPlaceholderText("Key")).toBeRequired();
    expect(screen.getByPlaceholderText("Value")).toBeRequired();
    expect(screen.getByRole("button", { name: "Remove environment variable" })).toBeDisabled();

    await user.type(screen.getByPlaceholderText("Value"), "token");
    expect(screen.getByPlaceholderText("Value")).toHaveValue("token");
  });
});
