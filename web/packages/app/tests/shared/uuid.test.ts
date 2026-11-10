import { createUUID } from "@/shared/lib/uuid";
import { webcrypto } from "node:crypto";

afterEach(() => vi.unstubAllGlobals());

describe("createUUID", () => {
  it("uses the native UUID generator when available", () => {
    const randomUUID = vi.fn(() => "30ac7e22-506e-4502-9650-38a23654c085");
    vi.stubGlobal("crypto", { randomUUID });
    expect(createUUID()).toBe("30ac7e22-506e-4502-9650-38a23654c085");
    expect(randomUUID).toHaveBeenCalledOnce();
  });

  it("generates distinct UUID v4 values on HTTP without randomUUID", () => {
    vi.stubGlobal("crypto", { getRandomValues: webcrypto.getRandomValues.bind(webcrypto) });
    const ids = Array.from({ length: 100 }, () => createUUID());
    for (const id of ids) {
      expect(id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
    }
    expect(new Set(ids).size).toBe(ids.length);
  });
});
