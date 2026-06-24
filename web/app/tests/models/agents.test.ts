import {
  advanceAgentProgress,
  agentCreateTemplateLocked,
  agentToDraft,
  applyTemplateToDraft,
  availableManagerRebuildRuntimeOptions,
  availableManagerRuntimeOptions,
  collectManagerTemplateVariants,
  defaultManagerRebuildImageForRuntime,
  agentDraftWithRuntimeFieldsFromAgent,
  agentRuntimePollSettled,
  agentStatusLabel,
  agentConnectedChannels,
  draftNotifierRuntimeOptionsForSave,
  draftRuntimeOptionsForSave,
  draftToProfile,
  ensureNotifierPullSubscriptionDraft,
  envRowsToMap,
  formatProviderLabel,
  hasConnectedAgentChannel,
  isAgentIncomplete,
  agentPageLLMProfileChanged,
  agentProfilePageSaveDisabled,
  isAgentProfileDraftComplete,
  mergeAgentIntoList,
  isNotificationBotAgent,
  mapToEnvRows,
  partitionWorkspaceAgentItems,
  notifierComputedPullRoutes,
  notifierFormIsComplete,
  notifierThirdPartyRelayWebhookURL,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
  notificationPushWebhookPathForBot,
  parseJSONMap,
  pickDefaultAgentTemplate,
  providerNeedsAuth,
  resolvedNotifierWebhookOrigin,
  resolveAgentChannelUserID,
  resolveAgentAvatarSource,
  runtimeImageForKind,
  runtimeOptionSchemasForAgent,
  localizedRuntimeOptionLabel,
  localizedRuntimeOptionDescription,
  shouldWaitForManagerRuntimeAfterProfileSave,
} from "@/models/agents";
import { AGENT_AVATAR_OPTIONS, selectUnusedAgentAvatar } from "@/shared/avatarOptions";

describe("agent model helpers", () => {
  it("normalizes agent profiles into editable drafts", () => {
    const draft = agentToDraft({
      agent_profile: {
        api_key_preview: "sk-...",
        api_key_set: true,
        env: { ZED: 1, alpha: "two" },
        headers: { "X-Trace": "1" },
        model_id: "qwen3",
        provider: "api",
        reasoning_effort: "",
        request_options: { temperature: 0.2 },
      },
      id: "worker-1",
      image: "worker:latest",
      instructions: "keep answers short",
      name: "Worker",
      role: "worker",
      runtime_kind: "openclaw_sandbox",
    });

    expect(draft).toMatchObject({
      agent_id: "worker-1",
      api_key_preview: "sk-...",
      api_key_set: true,
      image: "worker:latest",
      instructions: "keep answers short",
      model_id: "qwen3",
      name: "Worker",
      provider: "api",
      reasoning_effort: "medium",
      runtime_kind: "openclaw_sandbox",
    });
    expect(draft.envRows).toEqual([
      { key: "alpha", value: "two" },
      { key: "ZED", value: "1" },
    ]);
    expect(JSON.parse(draft.headersText)).toEqual({ "X-Trace": "1" });
    expect(JSON.parse(draft.requestOptionsText)).toEqual({ temperature: 0.2 });
  });

  it("converts env rows to maps while rejecting missing and duplicate keys", () => {
    expect(
      envRowsToMap([
        { key: " FOO ", value: "one" },
        { key: "", value: "" },
        { key: "BAR", value: "" },
      ]),
    ).toEqual({ FOO: "one" });

    expect(() => envRowsToMap([{ key: "", value: "set" }])).toThrow("Environment variable key is required");
    expect(() =>
      envRowsToMap([
        { key: "Path", value: "one" },
        { key: "PATH", value: "two" },
      ]),
    ).toThrow("Duplicate environment variable: PATH");
  });

  it("merges a fresh action response into the existing agent list", () => {
    expect(
      mergeAgentIntoList(
        [
          {
            id: "u-manager",
            image: "registry.example/opencsghq/picoclaw:2026.05.22",
            agent_profile: {
              image_upgrade_required: true,
              model_id: "gpt-5.5",
            },
            status: "running",
          },
          { id: "u-alice", image: "registry.example/worker:2026.06.03" },
        ],
        {
          id: "u-manager",
          image: "registry.example/opencsghq/picoclaw:2026.06.03",
          agent_profile: {
            image_upgrade_required: false,
          },
        },
      ),
    ).toEqual([
      {
        id: "u-manager",
        image: "registry.example/opencsghq/picoclaw:2026.06.03",
        agent_profile: {
          image_upgrade_required: false,
          model_id: "gpt-5.5",
        },
        status: "running",
      },
      { id: "u-alice", image: "registry.example/worker:2026.06.03" },
    ]);
  });

  it("syncs readonly runtime fields from a fresh action response into the agent page draft", () => {
    const draft = agentToDraft({
      id: "u-manager",
      image: "registry.example/opencsghq/picoclaw:2026.05.22",
      name: "manager",
      runtime_kind: "picoclaw_sandbox",
      agent_profile: {
        model_id: "gpt-5.5",
        provider: "codex",
      },
    });

    expect(
      agentDraftWithRuntimeFieldsFromAgent(draft, {
        id: "u-manager",
        image: "registry.example/opencsghq/picoclaw:2026.06.03",
        runtime_kind: "picoclaw_sandbox",
      }),
    ).toMatchObject({
      agent_id: "u-manager",
      image: "registry.example/opencsghq/picoclaw:2026.06.03",
      default_image: "registry.example/opencsghq/picoclaw:2026.06.03",
      model_id: "gpt-5.5",
      provider: "codex",
      runtime_kind: "picoclaw_sandbox",
    });
  });

  it("resolves CSGClaw DM identity from participant channel user before runtime agent id", () => {
    expect(
      resolveAgentChannelUserID({
        id: "u-manager",
        name: "manager",
        role: "manager",
        participants: [
          {
            agent_id: "u-manager",
            channel: "csgclaw",
            channel_user_ref: "manager",
            id: "manager",
          },
        ],
      }),
    ).toBe("manager");

    expect(
      resolveAgentChannelUserID({
        id: "u-worker",
        participants: [
          {
            agent_id: "u-worker",
            channel: "csgclaw",
            id: "worker",
          },
        ],
      }),
    ).toBe("worker");

    expect(resolveAgentChannelUserID({ id: "u-manager", role: "manager" })).toBe("manager");
    expect(resolveAgentChannelUserID({ id: "u-worker" })).toBe("u-worker");
  });

  it("detects app-backed Feishu agent channel connections from participants", () => {
    const item = {
      id: "u-dev",
      participants: [
        {
          agent_id: "u-dev",
          channel: "csgclaw",
          id: "dev",
          type: "agent",
        },
        {
          agent_id: "u-dev",
          channel: "feishu",
          channel_user_kind: "app_id",
          id: "dev",
          type: "agent",
        },
      ],
    };

    expect(agentConnectedChannels(item).map((channel) => channel.id)).toEqual(["feishu"]);
    expect(hasConnectedAgentChannel(item, "feishu")).toBe(true);
    expect(
      hasConnectedAgentChannel(
        { id: "u-dev", participants: [{ channel: "feishu", id: "admin", type: "human" }] },
        "feishu",
      ),
    ).toBe(false);
  });

  it("keeps JSON profile fields object-shaped", () => {
    expect(parseJSONMap("")).toEqual({});
    expect(parseJSONMap('{"temperature":0.1}')).toEqual({ temperature: 0.1 });
    expect(() => parseJSONMap("[1,2,3]")).toThrow("Expected a JSON object");

    expect(
      draftToProfile({
        api_key: "",
        api_key_preview: "",
        api_key_set: false,
        base_url: "http://127.0.0.1:11435/v1",
        enable_fast_mode: true,
        envRows: [{ key: "MODEL_HOME", value: "/models" }],
        headersText: '{"Authorization":"Bearer test"}',
        model_id: "Qwen/Qwen3-0.6B-GGUF",
        model_provider_id: "csghub-lite",
        provider: "csghub_lite",
        reasoning_effort: "",
        requestOptionsText: '{"top_p":0.9}',
        runtime_kind: "picoclaw_sandbox",
      }),
    ).toMatchObject({
      description: "Manager Worker Dispatch",
      enable_fast_mode: true,
      env: { MODEL_HOME: "/models" },
      headers: {},
      name: "manager",
      reasoning_effort: "medium",
      request_options: { top_p: 0.9 },
    });
  });

  it("does not serialize hidden OpenAPI fields for CSGHub Lite profiles", () => {
    expect(
      draftToProfile({
        api_key: "stale-key",
        api_key_preview: "",
        api_key_set: false,
        base_url: "https://api.deepseek.com",
        enable_fast_mode: false,
        envRows: [],
        headersText: '{"X-Stale":"1"}',
        model_id: "Qwen3-0.6B-GGUF",
        provider: "csghub_lite",
        reasoning_effort: "medium",
        requestOptionsText: "{}",
        runtime_kind: "codex",
      }),
    ).toMatchObject({
      provider: "csghub_lite",
      base_url: "",
      api_key: "",
      headers: {},
      model_id: "Qwen3-0.6B-GGUF",
    });

    expect(
      draftToProfile({
        api_key: "stale-key",
        api_key_preview: "",
        api_key_set: false,
        base_url: "https://api.deepseek.com",
        enable_fast_mode: false,
        envRows: [],
        headersText: '{"X-Stale":"1"}',
        model_id: "deepseek-v4-flash",
        provider: "csghub",
        reasoning_effort: "medium",
        requestOptionsText: "{}",
        runtime_kind: "picoclaw_sandbox",
      }),
    ).toMatchObject({
      provider: "csghub",
      base_url: "",
      api_key: "",
      headers: {},
      model_id: "deepseek-v4-flash",
    });
  });

  it("merges generic runtime options into the saved payload", () => {
    expect(
      draftRuntimeOptionsForSave({
        runtime_options: {
          ignored_empty: "   ",
          local_workspace_dir: "  /tmp/project  ",
        },
      }),
    ).toEqual({
      local_workspace_dir: "/tmp/project",
    });
    expect(
      draftRuntimeOptionsForSave({
        runtime_options: {
          local_workspace_dir: "   ",
        },
      }),
    ).toBeNull();

    expect(
      runtimeOptionSchemasForAgent("codex", null, {
        runtime_option_schemas: {
          codex: [
            {
              key: "local_workspace_dir",
              path: "local_workspace_dir",
              label: "Local Workspace Dir",
              label_zh: "本地工作目录",
              label_en: "Local Workspace Dir",
              description_zh: "留空时使用默认 Agent 工作目录。",
              description_en: "Leave empty to use the default agent workspace.",
            },
          ],
        },
      }),
    ).toEqual([
      {
        key: "local_workspace_dir",
        path: "local_workspace_dir",
        label: "Local Workspace Dir",
        label_zh: "本地工作目录",
        label_en: "Local Workspace Dir",
        description: "",
        description_zh: "留空时使用默认 Agent 工作目录。",
        description_en: "Leave empty to use the default agent workspace.",
        type: "text",
        required: false,
        picker: "",
        options: [],
      },
    ]);
    expect(
      localizedRuntimeOptionLabel(
        {
          path: "local_workspace_dir",
          label: "Local Workspace Dir",
          label_zh: "本地工作目录",
          label_en: "Local Workspace Dir",
        },
        "zh",
      ),
    ).toBe("本地工作目录");
    expect(
      localizedRuntimeOptionDescription(
        {
          path: "local_workspace_dir",
          description_zh: "留空时使用默认 Agent 工作目录。",
          description_en: "Leave empty to use the default agent workspace.",
        },
        "en",
      ),
    ).toBe("Leave empty to use the default agent workspace.");
  });

  it("selects runtime-specific templates and images", () => {
    const templates = [
      { id: "custom/worker", name: "custom-worker", runtime_kind: "picoclaw_sandbox" },
      { id: "builtin.openclaw-worker", name: "openclaw-worker", runtime_kind: "openclaw_sandbox" },
      { id: "builtin.picoclaw-worker", name: "picoclaw-worker", runtime_kind: "picoclaw_sandbox" },
    ];
    const bootstrapConfig = {
      default_worker_template: "custom/worker",
      effective_manager_image: "manager:effective",
      runtime_default_images: {
        openclaw_sandbox: "openclaw:worker",
        picoclaw_sandbox: "picoclaw:worker",
      },
      runtime_kind: "openclaw_sandbox",
    };

    expect(pickDefaultAgentTemplate(templates, "picoclaw_sandbox", bootstrapConfig)?.id).toBe("custom/worker");
    expect(pickDefaultAgentTemplate(templates, "openclaw_sandbox", bootstrapConfig)?.id).toBe(
      "builtin.openclaw-worker",
    );
    expect(pickDefaultAgentTemplate(templates, "notification", bootstrapConfig)).toBeNull();
    expect(runtimeImageForKind("openclaw_sandbox", bootstrapConfig, "fallback:worker")).toBe("openclaw:worker");
    expect(runtimeImageForKind("codex", bootstrapConfig, "fallback:worker")).toBe("");
    expect(runtimeImageForKind("notification", bootstrapConfig, "fallback:worker")).toBe("");

    expect(
      applyTemplateToDraft(
        {
          api_key: "",
          api_key_preview: "",
          api_key_set: false,
          base_url: "",
          enable_fast_mode: false,
          envRows: [],
          headersText: "{}",
          model_id: "",
          provider: "csghub_lite",
          reasoning_effort: "medium",
          requestOptionsText: "{}",
          runtime_kind: "picoclaw_sandbox",
        },
        templates[1],
        bootstrapConfig,
      ),
    ).toMatchObject({
      from_template: "builtin.openclaw-worker",
      image: "openclaw:worker",
      runtime_kind: "openclaw_sandbox",
      template_name: "openclaw-worker",
    });
  });

  it("applies template image_env contracts to draft env rows", () => {
    expect(
      applyTemplateToDraft(
        {
          api_key: "",
          api_key_preview: "",
          api_key_set: false,
          base_url: "",
          enable_fast_mode: false,
          envRows: [{ key: "OLD", value: "keep" }],
          headersText: "{}",
          model_id: "",
          provider: "csghub_lite",
          reasoning_effort: "medium",
          requestOptionsText: "{}",
          runtime_kind: "picoclaw_sandbox",
        },
        {
          id: "custom/gitlab",
          name: "gitlab-assistant",
          runtime_kind: "picoclaw_sandbox",
          image_env: [
            { name: "GITLAB_TOKEN", required: true, secret: true },
            { name: "GITLAB_URL", default: "https://gitlab.example.com" },
          ],
        },
        null,
      )?.envRows,
    ).toEqual([
      { key: "GITLAB_TOKEN", value: "" },
      { key: "GITLAB_URL", value: "https://gitlab.example.com" },
    ]);
  });

  it("filters manager rebuild runtime options to gateway runtimes", () => {
    expect(
      availableManagerRuntimeOptions({
        supported_runtime_kinds: ["picoclaw_sandbox", "openclaw_sandbox", "codex", "notifier", "picoclaw_sandbox"],
      }).map((option) => option.value),
    ).toEqual(["picoclaw_sandbox", "openclaw_sandbox"]);

    expect(availableManagerRuntimeOptions(null).map((option) => option.value)).toEqual([
      "picoclaw_sandbox",
      "openclaw_sandbox",
    ]);
  });

  it("prefers a user-set avatar over the agent avatar when available", () => {
    const usersById = new Map([
      ["u-gitlab", { id: "u-gitlab", avatar: "avatar/cartoon-3.png", name: "GitLab Assistant" }],
    ]);

    expect(
      resolveAgentAvatarSource(
        {
          id: "u-gitlab",
          avatar: "GI",
          name: "GitLab Assistant",
          role: "worker",
        },
        usersById,
      ),
    ).toBe("avatar/cartoon-3.png");
  });

  it("selects a built-in avatar that is not already used", () => {
    const availableAvatar = AGENT_AVATAR_OPTIONS.at(-1)?.value || "";
    const sources = AGENT_AVATAR_OPTIONS.slice(0, -1).map((option) => ({ avatar: option.value }));

    expect(selectUnusedAgentAvatar(sources)).toBe(availableAvatar);
  });

  it("uses manager template variants for manager rebuild runtime and default image", () => {
    const variants = collectManagerTemplateVariants([
      {
        id: "builtin.picoclaw-manager",
        role: "manager",
        runtime_kind: "picoclaw_sandbox",
        image: "picoclaw:manager",
      },
      {
        id: "builtin.openclaw-manager",
        role: "manager",
        runtime_kind: "openclaw_sandbox",
        image: "openclaw:manager",
      },
      {
        id: "builtin.openclaw-worker",
        role: "worker",
        runtime_kind: "openclaw_sandbox",
        image: "openclaw:worker",
      },
      {
        id: "duplicate.openclaw-manager",
        role: "manager",
        runtime_kind: "openclaw_sandbox",
        image: "openclaw:manager",
      },
    ]);

    expect(variants).toEqual([
      { runtimeKind: "picoclaw_sandbox", image: "picoclaw:manager" },
      { runtimeKind: "openclaw_sandbox", image: "openclaw:manager" },
    ]);
    expect(
      availableManagerRebuildRuntimeOptions(
        variants,
        { supported_runtime_kinds: ["codex", "picoclaw_sandbox"] },
        "custom_sandbox",
      ).map((option) => option.value),
    ).toEqual(["custom_sandbox", "picoclaw_sandbox", "openclaw_sandbox"]);
    expect(
      defaultManagerRebuildImageForRuntime(
        variants,
        "picoclaw_sandbox",
        { runtime_default_images: { picoclaw_sandbox: "picoclaw:latest" } },
        "picoclaw:old",
      ),
    ).toBe("picoclaw:manager");
    expect(defaultManagerRebuildImageForRuntime(variants, "openclaw_sandbox", null, "fallback:manager")).toBe(
      "openclaw:manager",
    );
    expect(
      defaultManagerRebuildImageForRuntime(
        [],
        "picoclaw_sandbox",
        {
          effective_manager_image: "picoclaw:effective-manager",
          runtime_default_images: { picoclaw_sandbox: "picoclaw:worker" },
          runtime_kind: "picoclaw_sandbox",
        },
        "fallback:manager",
      ),
    ).toBe("picoclaw:effective-manager");
    expect(defaultManagerRebuildImageForRuntime([], "openclaw_sandbox", null, "fallback:manager")).toBe(
      "fallback:manager",
    );
  });

  it("normalizes runtime and auth provider labels", () => {
    expect(normalizeRuntimeKind("codex")).toBe("codex");
    expect(notificationPushWebhookPathForBot("u-test")).toBe(
      "/api/v1/channels/csgclaw/participants/u-test/notifications",
    );
    expect(normalizeRuntimeKind("unknown")).toBe("unknown");
    expect(normalizeAuthProviderName("claude-code")).toBe("claude_code");
    expect(providerNeedsAuth("claude")).toBe(true);
    expect(providerNeedsAuth("api")).toBe(false);
    expect(formatProviderLabel("csghub_lite")).toBe("CSGHub Lite");
    expect(formatProviderLabel("csghub")).toBe("CSGHub");
  });

  it("advances agent creation progress toward each step target", () => {
    expect(advanceAgentProgress(null)).toBeNull();
    expect(
      advanceAgentProgress({
        index: 0,
        percent: 4,
        startedAt: 1,
        status: "running",
        steps: [{ label: "configure", target: 16 }],
      }),
    ).toMatchObject({ index: 0, percent: 8 });
    expect(
      advanceAgentProgress({
        index: 0,
        percent: 16,
        startedAt: 1,
        status: "running",
        steps: [
          { label: "configure", target: 16 },
          { label: "start", target: 88 },
        ],
      }),
    ).toMatchObject({ index: 1, percent: 16 });
  });

  it("maps non-object env values to one blank editable row", () => {
    expect(mapToEnvRows(null)).toEqual([{ key: "", value: "" }]);
    expect(mapToEnvRows(["not", "a", "map"])).toEqual([{ key: "", value: "" }]);
  });

  it("normalizes notification bot runtime options outside request options", () => {
    const draft = agentToDraft({
      id: "n-1",
      name: "Notifier",
      role: "worker",
      type: "notification",
      runtime_options: {
        delivery_mode: "remote_pull",
        remote_url: "https://relay.example.com/api/v1/inbox/messages",
        notification_profile: { delivery_complete: true, remote_token_set: true },
      },
    });

    expect(draft.bot_type).toBe("notification");
    expect(draft.notifier_delivery_mode).toBe("remote_pull");
    expect(draft.notifier_remote_url).toBe("https://relay.example.com/api/v1/inbox/messages");
    expect(draft.notifier_delivery_complete).toBe(true);
    expect(notifierFormIsComplete(draft)).toBe(true);
    expect(isAgentIncomplete({ type: "notification", runtime_options: {} })).toBe(true);
    expect(isAgentIncomplete({ type: "notification", available: true, runtime_options: {} })).toBe(false);
  });

  it("treats a saved profile draft as configured even when profile_complete is false", () => {
    const draft = agentToDraft({
      id: "u-manager",
      profile_complete: false,
      agent_profile: {
        profile_complete: false,
        provider: "api",
        model_provider_id: "openai",
        api_key_set: true,
        base_url: "https://api.example/v1",
        model_id: "glm-5.1",
      },
    });

    expect(
      isAgentIncomplete(
        { id: "u-manager", profile_complete: false, agent_profile: { profile_complete: false } },
        draft,
      ),
    ).toBe(false);
  });

  it("maps profile_incomplete status to offline label", () => {
    const t = (key: string) => key;
    expect(agentStatusLabel("profile_incomplete", t)).toBe("offline");
    expect(agentStatusLabel("running", t)).toBe("online");
  });

  it("waits for manager runtime only when profile save may bootstrap sandbox", () => {
    const runningManager = {
      id: "u-manager",
      box_id: "box-manager",
      profile_complete: true,
      status: "running",
    };
    const stoppedManager = { ...runningManager, status: "stopped" };

    expect(shouldWaitForManagerRuntimeAfterProfileSave(runningManager)).toBe(false);
    expect(shouldWaitForManagerRuntimeAfterProfileSave(stoppedManager)).toBe(false);
    expect(shouldWaitForManagerRuntimeAfterProfileSave(runningManager, { profileIncompleteBeforeSave: true })).toBe(
      true,
    );
    expect(shouldWaitForManagerRuntimeAfterProfileSave({ ...runningManager, box_id: "" })).toBe(true);
    expect(
      shouldWaitForManagerRuntimeAfterProfileSave({
        id: "u-manager",
        profile_complete: false,
        status: "profile_incomplete",
      }),
    ).toBe(true);
  });

  it("treats settled manager runtime states as complete for polling", () => {
    expect(
      agentRuntimePollSettled({
        id: "u-manager",
        box_id: "box-manager",
        status: "stopped",
      }),
    ).toBe(true);
    expect(
      agentRuntimePollSettled({
        id: "u-manager",
        box_id: "",
        profile_complete: true,
        status: "stopped",
      }),
    ).toBe(true);
    expect(
      agentRuntimePollSettled({
        id: "u-manager",
        box_id: "",
        profile_complete: false,
        status: "profile_incomplete",
      }),
    ).toBe(false);
  });

  it("allows meta-only agent page saves while LLM profile is incomplete", () => {
    const savedDraft = agentToDraft({
      id: "u-manager",
      name: "Manager",
      avatar: "avatar-a",
      description: "desc",
      provider: "api",
      profile_complete: false,
    });
    const avatarOnlyDraft = { ...savedDraft, avatar: "avatar-b" };
    const descriptionOnlyDraft = { ...savedDraft, description: "updated desc" };

    expect(agentPageLLMProfileChanged(avatarOnlyDraft, savedDraft)).toBe(false);
    expect(agentPageLLMProfileChanged(descriptionOnlyDraft, savedDraft)).toBe(false);
    expect(agentProfilePageSaveDisabled(avatarOnlyDraft, { id: "u-manager" }, { savedDraft })).toBe(false);
    expect(agentProfilePageSaveDisabled(descriptionOnlyDraft, { id: "u-manager" }, { savedDraft })).toBe(false);
  });

  it("blocks agent page saves when LLM profile fields change while incomplete", () => {
    const savedDraft = agentToDraft({
      id: "u-manager",
      name: "Manager",
      provider: "api",
      profile_complete: false,
    });
    const modelChangedDraft = { ...savedDraft, model_id: "gpt-test", base_url: "https://api.example.test/v1" };

    expect(agentPageLLMProfileChanged(modelChangedDraft, savedDraft)).toBe(true);
    expect(agentProfilePageSaveDisabled(modelChangedDraft, { id: "u-manager" }, { savedDraft })).toBe(true);
  });

  it("requires a catalog provider reference for model draft completeness", () => {
    expect(
      isAgentProfileDraftComplete({
        provider: "api",
        base_url: "https://api.example.test/v1",
        model_id: "gpt-test",
      }),
    ).toBe(false);
    expect(
      isAgentProfileDraftComplete({
        provider: "api",
        api_key_set: true,
        base_url: "https://api.example.test/v1",
        model_provider_id: "openai",
        model_id: "gpt-test",
      }),
    ).toBe(true);
  });

  it("locks runtime and image on create when a template is selected", () => {
    expect(agentCreateTemplateLocked({ from_template: "" }, "create")).toBe(false);
    expect(agentCreateTemplateLocked({ from_template: "builtin.picoclaw-worker" }, "create")).toBe(true);
    expect(agentCreateTemplateLocked({ from_template: "builtin.picoclaw-worker" }, "edit")).toBe(false);
  });

  it("builds notifier pull subscriptions and route previews", () => {
    const draft = ensureNotifierPullSubscriptionDraft({
      bot_type: "notification",
      notifier_delivery_mode: "remote_pull",
    }) as ReturnType<typeof agentToDraft>;

    expect(draft.notifier_remote_subscription_id).toMatch(/^sub-/);
    expect(
      draftNotifierRuntimeOptionsForSave({
        ...draft,
        notifier_remote_url: "https://relay.example.com/api/v1/webhooks/ingress",
        notifier_remote_token: "token",
      }),
    ).toMatchObject({
      delivery_mode: "remote_pull",
      remote_url: "https://relay.example.com/api/v1/webhooks/ingress",
      remote_token: "token",
    });
    expect(notifierComputedPullRoutes("https://relay.example.com/api/v1/webhooks/ingress")).toEqual({
      messages: "https://relay.example.com/api/v1/inbox/messages",
      ack: "https://relay.example.com/api/v1/inbox/ack",
    });
    expect(notifierThirdPartyRelayWebhookURL("https://relay.example.com/api/v1/inbox/messages", "sub-1")).toBe(
      "https://relay.example.com/api/v1/webhooks/ingress?subscription_id=sub-1",
    );
    expect(notifierThirdPartyRelayWebhookURL("http://opencsg-stg.com/api/v1/csgbot/notification-relay", "sub-1")).toBe(
      "http://opencsg-stg.com/api/v1/csgbot/notification-relay/webhooks/ingress?subscription_id=sub-1",
    );
    expect(notifierThirdPartyRelayWebhookURL("https://relay.example.com", "sub-1")).toBe(
      "https://relay.example.com/webhooks/ingress?subscription_id=sub-1",
    );
  });

  it("partitions workspace agents into workers and notification bots", () => {
    const agents = [
      { id: "u-manager", role: "manager", type: "normal" },
      { id: "u-worker", role: "worker", type: "normal" },
      { id: "u-notify", role: "worker", type: "notification" },
    ];
    const { workerAgentItems, notificationAgentItems } = partitionWorkspaceAgentItems(agents);
    expect(workerAgentItems.map((item) => item.id)).toEqual(["u-manager", "u-worker"]);
    expect(notificationAgentItems.map((item) => item.id)).toEqual(["u-notify"]);
    expect(isNotificationBotAgent({ type: "notification" })).toBe(true);
    expect(isNotificationBotAgent({ bot_type: "notification" })).toBe(true);
  });

  it("uses bootstrap advertise_base_url for notifier webhook origin", () => {
    expect(resolvedNotifierWebhookOrigin(null)).toBe("");
    expect(resolvedNotifierWebhookOrigin({ advertise_base_url: "http://127.0.0.1:18080/" })).toBe(
      "http://127.0.0.1:18080",
    );
  });
});
