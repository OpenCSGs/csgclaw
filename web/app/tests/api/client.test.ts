import { get } from "@/api/client";

describe("API client structured errors", () => {
  it("preserves stable error codes and safe fallback messages", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: {
            code: "model_unavailable",
            message: "The selected model is temporarily unavailable.",
          },
        }),
        { status: 503, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(get("api/v1/test")).rejects.toEqual({
      status: 503,
      code: "model_unavailable",
      message: "The selected model is temporarily unavailable.",
    });
  });
});
