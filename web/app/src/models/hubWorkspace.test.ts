import { describe, expect, it } from "vitest";
import {
  hubTemplateReviewState,
  isHubTemplateDeploySensitiveCheckError,
  isHubTemplateDeployReviewPendingError,
  isHubTemplateSensitiveInformationError,
  mergeHubTemplateDetail,
  upsertHubTemplateReviewState,
  type HubTemplate,
} from "./hubWorkspace";

function templateWithStatus(status: string, messages: string[] = []): HubTemplate {
  return {
    id: "Agentic/reviewer",
    name: "reviewer",
    metadata: {
      sensitive_check: {
        status,
        failure_details: messages.map((message) => ({ message })),
      },
    },
  };
}

describe("hubTemplateReviewState", () => {
  it.each(["Pass", "Skip", ""])("hides %s review status", (status) => {
    expect(hubTemplateReviewState(templateWithStatus(status))).toBeNull();
  });

  it("returns a pending state", () => {
    expect(hubTemplateReviewState(templateWithStatus("Pending"))).toEqual({ kind: "pending", messages: [] });
  });

  it("returns exception failure messages", () => {
    expect(hubTemplateReviewState(templateWithStatus("Exception", ["secret detected", " "]))).toEqual({
      kind: "exception",
      messages: ["secret detected"],
    });
  });

  it("treats the repository API Fail status as a failed review", () => {
    const message =
      'label:political_content,reason:{"riskLevel":"high","riskTips":"涉政_首长同音,涉政_敏感人物_领导人","riskWords":"习**"},requestId:01A00F33';
    expect(hubTemplateReviewState(templateWithStatus("Fail", [message]))).toEqual({
      kind: "exception",
      messages: [message],
    });
  });
});

describe("mergeHubTemplateDetail", () => {
  it("keeps sensitive-check metadata from the list when detail omits it", () => {
    const summary = templateWithStatus("Fail", ["checker result"]);
    expect(mergeHubTemplateDetail({ ...summary, metadata: null }, summary)?.metadata).toEqual(summary.metadata);
  });
});

describe("upsertHubTemplateReviewState", () => {
  it("creates a selectable pending template when the refreshed catalog does not contain it yet", () => {
    const templates = upsertHubTemplateReviewState([], "Agentic/reviewer", "Pending");

    expect(templates[0]).toMatchObject({
      id: "Agentic/reviewer",
      namespace: "Agentic",
      name: "reviewer",
      role: "worker",
      source: { kind: "remote", name: "official" },
    });
    expect(hubTemplateReviewState(templates[0])).toEqual({ kind: "pending", messages: [] });
    expect(mergeHubTemplateDetail({ id: "Agentic/reviewer", name: "reviewer" }, templates[0])).toMatchObject({
      metadata: { sensitive_check: { status: "Pending" } },
    });
  });

  it("adds failed review metadata without discarding catalog fields", () => {
    const templates = upsertHubTemplateReviewState(
      [{ id: "Agentic/reviewer", name: "Reviewer", description: "catalog description" }],
      "Agentic/reviewer",
      "Fail",
      "review failed",
    );

    expect(templates[0]).toMatchObject({ name: "Reviewer", description: "catalog description" });
    expect(hubTemplateReviewState(templates[0])).toEqual({ kind: "exception", messages: ["review failed"] });
  });
});

describe("isHubTemplateSensitiveInformationError", () => {
  it("recognizes the normalized backend error", () => {
    expect(
      isHubTemplateSensitiveInformationError({ status: 400, message: "template contains sensitive information" }),
    ).toBe(true);
  });

  it("recognizes the upstream error code for compatibility", () => {
    expect(isHubTemplateSensitiveInformationError({ status: 400, message: '{"code":"SENSITIVE-ERR-0"}' })).toBe(true);
  });
});

describe("isHubTemplateDeploySensitiveCheckError", () => {
  it("recognizes the normalized deployment error", () => {
    expect(
      isHubTemplateDeploySensitiveCheckError({
        status: 409,
        message:
          "template published but deployment failed: community template has not passed the sensitive-content check",
      }),
    ).toBe(true);
  });

  it("recognizes the upstream deployment error code", () => {
    expect(isHubTemplateDeploySensitiveCheckError({ status: 409, message: '{"code":"AGENT-ERR-23"}' })).toBe(true);
  });
});

describe("isHubTemplateDeployReviewPendingError", () => {
  it("recognizes the structured pending-review response", () => {
    expect(isHubTemplateDeployReviewPendingError({ code: "AGENT-ERR-22", message: "review pending" })).toBe(true);
  });
});
