import { describe, expect, it } from "vitest";
import {
  HubTemplateErrorCodes,
  hubTemplateErrorCode,
  hubTemplateReviewState,
  mergeHubTemplateDetail,
  upsertHubTemplateReviewState,
  type HubTemplate,
} from "./hubWorkspace";

function templateWithStatus(status: string, details: Array<{ path?: string; message?: string }> = []): HubTemplate {
  return {
    id: "Agentic/reviewer",
    name: "reviewer",
    metadata: {
      sensitive_check: {
        status,
        failure_details: details,
      },
    },
  };
}

describe("hubTemplateReviewState", () => {
  it.each(["Pass", "Skip", ""])("hides %s review status", (status) => {
    expect(hubTemplateReviewState(templateWithStatus(status))).toBeNull();
  });

  it("returns a pending state", () => {
    expect(hubTemplateReviewState(templateWithStatus("Pending"))).toEqual({ kind: "pending", paths: [] });
  });

  it("returns sensitive file paths without exposing checker messages", () => {
    expect(
      hubTemplateReviewState(
        templateWithStatus("Exception", [
          { path: "skills/review.md", message: "secret detected" },
          { path: " ", message: "another checker message" },
        ]),
      ),
    ).toEqual({
      kind: "exception",
      paths: ["skills/review.md"],
    });
  });

  it("treats the repository API Fail status as a failed review", () => {
    const message =
      'label:political_content,reason:{"riskLevel":"high","riskTips":"涉政_首长同音,涉政_敏感人物_领导人","riskWords":"习**"},requestId:01A00F33';
    expect(hubTemplateReviewState(templateWithStatus("Fail", [{ path: "AGENTS.md", message }]))).toEqual({
      kind: "exception",
      paths: ["AGENTS.md"],
    });
  });
});

describe("mergeHubTemplateDetail", () => {
  it("keeps sensitive-check metadata from the list when detail omits it", () => {
    const summary = templateWithStatus("Fail", [{ path: "README.md", message: "checker result" }]);
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
    expect(hubTemplateReviewState(templates[0])).toEqual({ kind: "pending", paths: [] });
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
    expect(hubTemplateReviewState(templates[0])).toEqual({ kind: "exception", paths: [] });
  });

  it("preserves server failure paths when updating review state", () => {
    const templates = upsertHubTemplateReviewState(
      [
        {
          id: "Agentic/reviewer",
          name: "Reviewer",
          metadata: {
            sensitive_check: {
              status: "Fail",
              failure_details: [
                {
                  path: "instructions/AGENTS.md",
                  message: "checker message",
                },
              ],
            },
          },
        },
      ],
      "Agentic/reviewer",
      "Fail",
      "review failed",
    );

    expect(hubTemplateReviewState(templates[0])).toEqual({
      kind: "exception",
      paths: ["instructions/AGENTS.md"],
    });
  });
});

describe("hubTemplateErrorCode", () => {
  it("reads stable structured publishing and deployment codes", () => {
    expect(hubTemplateErrorCode({ code: "AGENT-ERR-22", message: "review pending" })).toBe(
      HubTemplateErrorCodes.reviewPending,
    );
    expect(hubTemplateErrorCode({ code: "RESOURCE-ERR-1" })).toBe(HubTemplateErrorCodes.deployResourceUnavailable);
  });

  it("does not infer codes from legacy error messages", () => {
    expect(hubTemplateErrorCode({ message: '{"code":"AGENT-ERR-23"}' })).toBe("");
  });
});
