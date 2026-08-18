import { apiErrorBillingURL, get, type ApiError } from "@/api/client";

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

  it("preserves and validates a structured billing URL", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: {
            code: "insufficient_balance",
            message: "Insufficient balance.",
            billing_url: "https://opencsg-stg.com/settings/billing",
          },
        }),
        { status: 402, headers: { "Content-Type": "application/json" } },
      ),
    );

    const error = await get("api/v1/test").catch((reason) => reason as ApiError);
    expect(apiErrorBillingURL(error)).toBe("https://opencsg-stg.com/settings/billing");
    expect(apiErrorBillingURL({ billingURL: "javascript:alert(1)" })).toBe("");
  });
});
