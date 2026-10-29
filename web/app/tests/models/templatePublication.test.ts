import { describe, expect, it } from "vitest";
import {
  templatePublicationStatus,
  templatePublicationTarget,
  templateVersionLabel,
} from "@/models/templatePublication";
import type { HubTemplate } from "@/models/hubWorkspace";

const local: HubTemplate = {
  id: "local.renamed",
  name: "renamed",
  uid: "identity",
  revision: 3,
  source: { kind: "local" },
  publications: [
    {
      site: "https://hub.example",
      namespace: "alice",
      template_id: "alice/original",
      revision: 2,
      published_at: "2026-10-29T00:00:00Z",
    },
  ],
};

describe("template publication status", () => {
  it("uses the saved association after a local rename, scoped to site and account", () => {
    expect(templatePublicationTarget(local, [], "alice", "https://hub.example/login")?.id).toBe("alice/original");
    expect(templatePublicationTarget(local, [], "bob", "https://hub.example")).toBeNull();
    expect(templatePublicationTarget(local, [], "alice", "https://other.example")).toBeNull();
    expect(
      templatePublicationTarget({ ...local, source: { kind: "remote" } }, [], "alice", "https://hub.example"),
    ).toBeNull();
  });
  it.each([
    [3, "identity", "current"],
    [2, "identity", "different"],
    [4, "identity", "different"],
    [3, "unrelated", "unknown"],
    [0, "identity", "unknown"],
  ] as const)("compares remote revision %s with identity %s", (revision, uid, expected) => {
    expect(templatePublicationStatus(local, { ...local, revision, uid })).toBe(expected);
  });
  it("does not equate a last publication record with a verified remote version", () => {
    expect(templatePublicationStatus(local)).toBe("unknown");
    expect(templatePublicationStatus({ ...local, revision: 2 })).toBe("unknown");
  });
  it("finds legacy same-name repositories without inferring identity", () => {
    const remote: HubTemplate = { id: "alice/renamed", name: "renamed", source: { kind: "remote" } };
    expect(
      templatePublicationTarget({ ...local, publications: [] }, [remote], "alice", "https://hub.example")?.id,
    ).toBe(remote.id);
    expect(templatePublicationStatus(local, remote)).toBe("unknown");
  });
  it("formats revisions and supports legacy version strings", () => {
    expect(templateVersionLabel(local)).toBe("V3");
    expect(templateVersionLabel({ ...local, revision: 0, version: "v1.2" })).toBe("v1.2");
    expect(templateVersionLabel(null)).toBe("");
  });
});
