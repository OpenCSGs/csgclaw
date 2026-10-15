import { act, renderHook } from "@testing-library/react";
import { checkModelProvider, createModelProvider } from "@/api/modelProviders";
import { createTranslator } from "@/shared/i18n";
import type { ModelProvider, ModelProviderCatalog } from "@/models/modelProviders";
import { useModelProviderCreation } from "./useModelProviderCreation";

vi.mock("@/api/modelProviders", () => ({ createModelProvider: vi.fn(), checkModelProvider: vi.fn() }));

const provider: ModelProvider = {
  id: "custom",
  kind: "custom",
  display_name: "Custom",
  preset: "custom",
  builtin: false,
  models: ["saved"],
  status: "unknown",
  api_key_set: true,
};
const refreshedProvider = { ...provider, models: ["discovered"], status: "connected" };
const catalog: ModelProviderCatalog = {
  providers: [refreshedProvider],
  customProviders: [refreshedProvider],
  builtinProviders: [],
};
const payload = { base_url: "https://models.example.test/v1" };

beforeEach(() => {
  vi.resetAllMocks();
});

function setup(keepCurrentPage = false) {
  const refresh = vi.fn().mockResolvedValue(catalog);
  const onSelect = vi.fn();
  const hook = renderHook(() =>
    useModelProviderCreation({
      catalog: null,
      locale: "en",
      refresh,
      onSelect,
      keepCurrentPage,
      t: createTranslator("en"),
    }),
  );
  act(() => hook.result.current.open());
  return { ...hook, refresh, onSelect };
}

test("connecting refreshes the catalog and selects the provider without a second access check", async () => {
  vi.mocked(createModelProvider).mockResolvedValue(provider);
  vi.mocked(checkModelProvider).mockRejectedValue(new Error("temporarily unavailable"));
  const { result, refresh, onSelect } = setup();
  await act(async () => {
    await result.current.modalProps!.onCreate(payload);
  });
  expect(checkModelProvider).not.toHaveBeenCalled();
  expect(refresh).toHaveBeenCalledTimes(1);
  expect(onSelect).toHaveBeenCalledWith(refreshedProvider);
  expect(result.current.modalProps).toBeNull();
});

test("creation from settings keeps the current page", async () => {
  vi.mocked(createModelProvider).mockResolvedValue(provider);
  const { result, onSelect, refresh } = setup(true);
  await act(async () => {
    await result.current.modalProps!.onCreate({ display_name: "Custom" });
  });
  expect(refresh).toHaveBeenCalledTimes(1);
  expect(onSelect).not.toHaveBeenCalled();
  expect(checkModelProvider).not.toHaveBeenCalled();
  expect(result.current.modalProps).toBeNull();
});

test("failed saves keep the dialog open and allow a retry", async () => {
  vi.mocked(createModelProvider).mockRejectedValueOnce(new Error("failed")).mockResolvedValueOnce(provider);
  const { result, refresh } = setup();
  await act(async () => {
    await result.current.modalProps!.onCreate(payload);
  });
  expect(result.current.modalProps?.error).toBe("failed");
  expect(result.current.modalProps?.busy).toBe(false);
  expect(refresh).not.toHaveBeenCalled();
  await act(async () => {
    await result.current.modalProps!.onCreate(payload);
  });
  expect(result.current.modalProps).toBeNull();
});

test("same-tick double submission and closing while a save is pending are blocked", async () => {
  let complete!: (value: ModelProvider) => void;
  vi.mocked(createModelProvider).mockReturnValue(
    new Promise((resolve) => {
      complete = resolve;
    }),
  );
  const { result } = setup();
  const modal = result.current.modalProps!;
  let pending!: Promise<void>;
  act(() => {
    pending = modal.onCreate({ display_name: "Custom" });
    void modal.onCreate({ display_name: "Custom" });
    modal.onClose();
  });
  expect(createModelProvider).toHaveBeenCalledTimes(1);
  expect(result.current.modalProps?.busy).toBe(true);
  await act(async () => {
    complete(provider);
    await pending;
  });
  expect(result.current.modalProps).toBeNull();
});
