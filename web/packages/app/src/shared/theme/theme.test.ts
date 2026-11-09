import { detectInitialTheme, parseThemeMode, resolveThemeMode } from "./theme";
import { THEME_STORAGE_KEY } from "@/shared/storage/keys";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

test("theme preferences are validated and restricted storage does not crash initialization", () => {
  expect(parseThemeMode("system")).toBe("system");
  expect(parseThemeMode("invalid")).toBe("dark");
  window.localStorage.setItem(THEME_STORAGE_KEY, "light");
  expect(detectInitialTheme()).toBe("light");
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("Restricted storage");
  });
  expect(detectInitialTheme()).toBe("dark");
});

test.each([false, true])("runtime resolves the system preference (dark=%s)", (dark) => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({ matches: dark })),
  );
  expect(resolveThemeMode("system")).toBe(dark ? "dark" : "light");
});
