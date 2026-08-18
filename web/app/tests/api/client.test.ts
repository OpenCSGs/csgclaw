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

  it("preserves partial template deployment fields", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: {
            code: "AGENT-ERR-23",
            message: "deployment failed",
            published_template_id: "alice/reviewer",
          },
        }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );

    const error = await get("api/v1/hub/templates").catch((caught: ApiError) => caught);
    expect(error).toMatchObject({
      status: 409,
      code: "AGENT-ERR-23",
      message: "deployment failed",
      publishedTemplateId: "alice/reviewer",
    });
  });
});
