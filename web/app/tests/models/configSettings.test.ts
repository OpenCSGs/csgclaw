import { describe, expect, it } from "vitest";
import {
  configAdvertiseBaseURLPlaceholder,
  configDraftToUpdatePayload,
  configSettingsToDraft,
  configTemplateOptions,
  formatListenAddress,
  isValidConfigBootstrapTemplate,
  normalizeConfigSettings,
  parseListenAddress,
} from "@/models/configSettings";

describe("configSettings model", () => {
  it("normalizes settings payload with masked token metadata", () => {
    const got = normalizeConfigSettings({
      path: "/tmp/config.toml",
      listen_addr: "127.0.0.1:18080",
      access_token_set: true,
      access_token_preview: "your...",
      show_upgrade: false,
      sandbox_provider: "docker",
      supported_sandbox_providers: ["boxlite", "docker", "csghub"],
      hub_local_path: "/tmp/hub",
      hub_official_url_effective: "https://hub.example.com/",
      default_manager_template: "builtin.manager-codex",
      default_worker_template: "builtin.picoclaw-worker",
    });
    expect(got?.access_token_set).toBe(true);
    expect(got?.access_token_preview).toBe("your...");
    expect(got?.access_token).toBe("");
    expect(got?.hub_local_path).toBe("/tmp/hub");
    expect(got?.hub_official_url_effective).toBe("https://hub.example.com");
  });

  it("parses and formats listen addresses", () => {
    expect(parseListenAddress("0.0.0.0:18080")).toEqual({ host: "0.0.0.0", port: "18080" });
    expect(formatListenAddress("0.0.0.0", "19080")).toBe("0.0.0.0:19080");
  });

  it("builds update payload without empty access token", () => {
    const draft = configSettingsToDraft(
      normalizeConfigSettings({
        path: "/tmp/config.toml",
        listen_addr: "0.0.0.0:18080",
        access_token_set: true,
        access_token_preview: "secr...",
        sandbox_provider: "boxlite",
        hub_local_path: "/tmp/hub",
        default_manager_template: "builtin.manager-codex",
        default_worker_template: "builtin.picoclaw-worker",
      })!,
    );
    draft.listen_port = "19080";
    expect(configDraftToUpdatePayload(draft)).toEqual({
      listen_addr: "0.0.0.0:19080",
      advertise_base_url: "",
      show_upgrade: true,
      sandbox_provider: "boxlite",
      hub_local_path: "/tmp/hub",
      default_manager_template: "builtin.manager-codex",
      default_worker_template: "builtin.picoclaw-worker",
    });
  });

  it("normalizes advertise base url on save", () => {
    const draft = configSettingsToDraft(
      normalizeConfigSettings({
        path: "/tmp/config.toml",
        listen_addr: "0.0.0.0:18080",
        advertise_base_url: "http://192.168.1.10:18080/",
        sandbox_provider: "boxlite",
        hub_local_path: "/tmp/hub",
        default_manager_template: "builtin.manager-codex",
        default_worker_template: "builtin.picoclaw-worker",
      })!,
    );
    draft.advertise_base_url = "http://example.test/base/";
    expect(configDraftToUpdatePayload(draft).advertise_base_url).toBe("http://example.test/base");
  });

  it("uses effective service url as placeholder when config value is empty", () => {
    const draft = configSettingsToDraft(
      normalizeConfigSettings({
        path: "/tmp/config.toml",
        listen_addr: "0.0.0.0:18080",
        advertise_base_url_effective: "http://192.168.1.10:18080",
        sandbox_provider: "boxlite",
        hub_local_path: "/tmp/hub",
        default_manager_template: "builtin.manager-codex",
        default_worker_template: "builtin.picoclaw-worker",
      })!,
    );
    expect(configAdvertiseBaseURLPlaceholder(draft)).toBe("http://192.168.1.10:18080");
    draft.advertise_base_url = "http://example.test";
    expect(configAdvertiseBaseURLPlaceholder(draft)).toBe("");
  });

  it("filters hub templates by role", () => {
    const options = configTemplateOptions(
      [
        { id: "builtin.manager-codex", name: "Manager", role: "manager", runtime_kind: "codex" },
        { id: "builtin.picoclaw-worker", name: "Worker", role: "worker", runtime_kind: "picoclaw_sandbox" },
      ],
      "manager",
      "builtin.manager-codex",
    );
    expect(options).toEqual([{ value: "builtin.manager-codex", label: "Manager" }]);
  });

  it("filters bootstrap templates by runtime_kind", () => {
    expect(
      isValidConfigBootstrapTemplate(
        { id: "builtin.manager-codex", role: "manager", runtime_kind: "picoclaw_sandbox" },
        "manager",
      ),
    ).toBe(false);
    expect(
      isValidConfigBootstrapTemplate(
        { id: "builtin.manager-codex", role: "manager", runtime_kind: "codex" },
        "manager",
      ),
    ).toBe(true);
    expect(
      isValidConfigBootstrapTemplate(
        { id: "builtin.manager-codex", role: "manager", runtime_kind: "openclaw_sandbox" },
        "manager",
      ),
    ).toBe(false);
    expect(isValidConfigBootstrapTemplate({ id: "bad-worker", role: "worker", runtime_kind: "" }, "worker")).toBe(
      false,
    );
    expect(
      isValidConfigBootstrapTemplate(
        { id: "builtin.picoclaw-worker", role: "worker", runtime_kind: "picoclaw_sandbox" },
        "worker",
      ),
    ).toBe(true);

    const workerOptions = configTemplateOptions(
      [
        { id: "builtin.picoclaw-worker", name: "Worker", role: "worker", runtime_kind: "picoclaw_sandbox" },
        { id: "bad-worker", name: "Bad", role: "worker", runtime_kind: "" },
      ],
      "worker",
      "builtin.picoclaw-worker",
    );
    expect(workerOptions).toEqual([{ value: "builtin.picoclaw-worker", label: "Worker" }]);
  });
});
