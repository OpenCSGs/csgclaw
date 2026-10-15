import { isOfficialTemplate, templateInGroup, templateSourceLabelKey } from "@/models/templateGroups";
import type { HubTemplate } from "@/models/hubWorkspace";

const official: HubTemplate = { id: "agentichub/a", name: "a", namespace: "agentichub", source: { kind: "remote" } };
const local: HubTemplate = { id: "local.a", name: "a", source: { kind: "local" } };
const own: HubTemplate = { id: "alice/a", name: "a", namespace: "alice", source: { kind: "remote" } };
const publicTemplate: HubTemplate = { id: "bob/a", name: "a", namespace: "bob", source: { kind: "remote" } };

describe("template groups", () => {
  it("identifies official namespaces and builtins without counting templates", () => {
    expect(isOfficialTemplate(official)).toBe(true);
    expect(isOfficialTemplate({ source: { kind: "builtin" } })).toBe(true);
    expect(isOfficialTemplate(own)).toBe(false);
    expect(isOfficialTemplate({ ...official, namespace: undefined })).toBe(true);
  });
  it("shows only local templates in My templates when signed out", () => {
    expect([official, local, own, publicTemplate].filter((t) => templateInGroup(t, "mine"))).toEqual([local]);
  });
  it("separates my templates and other public templates when signed in", () => {
    const templates = [official, local, own, publicTemplate];
    expect(templates.filter((t) => templateInGroup(t, "mine", "alice"))).toEqual([local, own]);
    expect(templates.filter((t) => templateInGroup(t, "official", "alice"))).toEqual([official]);
    expect(templates.filter((t) => templateInGroup(t, "public", "alice"))).toEqual([publicTemplate]);
    expect(templates.filter((t) => templateInGroup(t, "public"))).toEqual([own, publicTemplate]);
  });
});

it("labels ownership independently of the remote registry name", () => {
  expect(templateSourceLabelKey({ ...own, source: { kind: "remote", name: "official" } }, "alice")).toBe(
    "taskEntryOverview_personal",
  );
  expect(templateSourceLabelKey({ ...publicTemplate, source: { kind: "remote", name: "official" } }, "alice")).toBe(
    "resourcesPublicTemplates",
  );
  expect(templateSourceLabelKey(official, "alice")).toBe("taskEntryOverview_official");
  expect(templateSourceLabelKey(local, "alice")).toBe("resourcesSkillLocalFilter");
});
