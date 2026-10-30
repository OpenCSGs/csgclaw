import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { UiLab } from "../UiLab";
import { UI_LAB_LOCALE_STORAGE_KEY, LOCALE_STORAGE_KEY } from "@/shared/storage/keys";
import { labMessages, labMessageKeys } from "@/shared/i18n/uiLab";
import { createLabText } from "./translation";

describe("UI Lab localization", () => {
  beforeEach(() => {
    localStorage.clear();
    window.history.replaceState(null, "", "#toggle-foundations");
  });

  it("switches descriptions, navigation, labels and status without losing edits, state or location", async () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, "zh");
    const user = userEvent.setup();
    const view = render(<UiLab />);
    const demo = within(screen.getByRole("region", { name: "开关交互试用" }));
    await user.click(demo.getByRole("switch", { name: "记住我" }));
    await user.clear(demo.getByRole("textbox", { name: "开关标题" }));
    await user.type(demo.getByRole("textbox", { name: "开关标题" }), "My custom label");
    await user.click(screen.getByRole("radio", { name: "English" }));
    expect(screen.getByRole("heading", { name: "Toggles" })).toBeVisible();
    const englishDemo = within(screen.getByRole("region", { name: "Toggle playground" }));
    expect(englishDemo.getByRole("switch", { name: "My custom label" })).toBeChecked();
    expect(englishDemo.getByRole("textbox", { name: "Toggle label" })).toHaveValue("My custom label");
    expect(englishDemo.getByRole("status")).toHaveTextContent("Current: On");
    expect(screen.getByRole("link", { name: "Toggles" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByText(/Toggles \(also known as/)).toBeVisible();
    expect(window.location.hash).toBe("#toggle-foundations");
    expect(document.documentElement.lang).toBe("en");
    expect(localStorage.getItem(UI_LAB_LOCALE_STORAGE_KEY)).toBe("en");
    expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBe("zh");
    const search = screen.getByRole("searchbox", { name: "Search component directory" });
    await user.type(search, "复选框");
    expect(screen.getByRole("link", { name: "Checkboxes" })).toBeVisible();
    await user.clear(search);
    await user.click(screen.getByRole("radio", { name: "中文" }));
    expect(screen.getByRole("textbox", { name: "开关标题" })).toHaveValue("My custom label");
    expect(screen.getByText(/开关（也称/)).toBeVisible();
    await user.click(screen.getByRole("radio", { name: "English" }));
    view.unmount();
    render(<UiLab />);
    await waitFor(() => expect(screen.getByRole("radio", { name: "English" })).toBeChecked());
    expect(screen.getByRole("heading", { name: "Toggles" })).toBeVisible();
  }, 15000);

  it("has matching catalogs and translates every registered source without exposing message keys", () => {
    expect(Object.keys(labMessages.zh).sort()).toEqual(Object.keys(labMessages.en).sort());
    for (const locale of ["zh", "en"] as const) {
      const text = createLabText(locale);
      for (const [source, key] of Object.entries(labMessageKeys)) {
        expect(text(source), `${locale}: ${source}`).toBe(labMessages[locale][key as keyof typeof labMessages.zh]);
        expect(text(source)).not.toMatch(/^uiLab\.copy\d+$/);
      }
    }
  });
});
