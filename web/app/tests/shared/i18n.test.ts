import {
  createTranslator,
  localizeAPIError,
  localizeTemplatePublishError,
  localizeTemplateSourceTag,
} from "@/shared/i18n";
import { messages } from "@/shared/i18n/messages";

describe("i18n messages", () => {
  it("keeps the human profile subtitle concise", () => {
    expect(createTranslator("en")("humanDetailSubtitle")).toBe("How you appear in chats, mentions, and collaboration.");
    expect(createTranslator("zh")("humanDetailSubtitle")).toBe("你在聊天、提及和协作中的显示方式。");
  });

  it("uses Connector terminology while retaining App API translation keys", () => {
    const labels = {
      en: ["Connectors", "Add connector", "Managed by connector", "Manage global Connectors"],
      zh: ["连接器", "添加连接器", "由连接器管理", "管理全局连接器"],
    } as const;
    for (const locale of ["en", "zh"] as const) {
      const t = createTranslator(locale);
      expect([t("agentAppsTab"), t("appAdd"), t("appManagedMCP"), t("appManageResources")]).toEqual(labels[locale]);
      expect(t("appSecretEnvironment")).toContain("App Secret");
    }
  });

  it("localizes the installed remote skill action", () => {
    expect(createTranslator("en")("resourcesSkillRemoteReplaceAction")).toBe("Replace");
    expect(createTranslator("zh")("resourcesSkillRemoteReplaceAction")).toBe("替换");
  });

  it("localizes personal Hub source tags", () => {
    expect(localizeTemplateSourceTag("personal", "zh")).toBe("个人");
    expect(localizeTemplateSourceTag("personal", "en")).toBe("personal");
  });

  it("localizes structured API errors by stable code", () => {
    expect(
      localizeAPIError(
        { status: 403, code: "knowledge_base_access_forbidden", message: "knowledge base access is not permitted" },
        createTranslator("zh"),
      ),
    ).toBe("当前账号无权访问该知识库。");
    expect(
      localizeAPIError(
        { status: 503, code: "model_unavailable", message: "The selected model is unavailable." },
        createTranslator("zh"),
      ),
    ).toContain("当前模型暂时不可用");
    expect(
      localizeAPIError({ status: 503, code: "model_unavailable", message: "服务不可用" }, createTranslator("en")),
    ).toContain("temporarily unavailable");
    expect(
      localizeAPIError(
        { status: 503, code: "RESOURCE-ERR-1", message: "The resource is temporarily unavailable." },
        createTranslator("zh"),
      ),
    ).toBe("模板已成功发布，但社区部署资源暂时不可用，请稍后重试部署。");
    expect(
      localizeAPIError(
        { status: 400, code: "SYS-ERR-4", message: "The repository already exists." },
        createTranslator("en"),
      ),
    ).toContain("community repository already exists");
    const pendingSensitiveCheck = { status: 409, code: "AGENT-ERR-25", message: "sensitive check pending" };
    expect(localizeAPIError(pendingSensitiveCheck, createTranslator("zh"))).toBe(
      "该智能体模板的敏感内容检查仍在进行中，无法创建智能体。",
    );
    expect(localizeAPIError(pendingSensitiveCheck, createTranslator("en"))).toBe(
      "This agent template is still undergoing sensitive-content review, so the agent cannot be created.",
    );
  });

  it("keeps error translation keys machine-readable", () => {
    expect(Object.keys(messages.zh.errors)).toEqual(Object.keys(messages.en.errors));
    expect(Object.keys(messages.zh.errors).filter((key) => /\s/.test(key))).toEqual([]);
  });

  it.each([
    ["zh", "file_reference_unavailable", "引用文件不可用，请检查文件是否存在以及来源 Agent 是否仍在会话中。"],
    [
      "en",
      "file_reference_unavailable",
      "Referenced file is unavailable. Check that it exists and its source agent is still in this conversation.",
    ],
    ["zh", "workspace_file_unavailable", "工作区文件不可用。"],
    ["en", "workspace_file_unavailable", "Workspace file is unavailable."],
  ] as const)("keeps the %s %s error distinct from the generic fallback", (locale, code, expected) => {
    expect(localizeAPIError({ status: 400, code, message: "file read failed" }, createTranslator(locale))).toBe(
      expected,
    );
  });

  it("hides English diagnostics for generic template publishing failures in Chinese", () => {
    const message =
      'publish hub template to "official": remote hub request failed with status 500: failed to update repository path';
    expect(localizeAPIError({ status: 400, code: "template_publish_failed", message }, createTranslator("zh"))).toBe(
      "模板发布失败，请稍后重试。",
    );
    expect(localizeAPIError({ status: 400, code: "template_publish_failed", message }, createTranslator("en"))).toBe(
      message,
    );
  });

  it("uses a Chinese fallback for unknown English API messages", () => {
    expect(localizeAPIError({ status: 500, message: "remote hub request failed" }, createTranslator("zh"))).toBe(
      "操作失败，请稍后重试。",
    );
    expect(
      localizeAPIError(
        { status: 500, message: "remote hub request failed" },
        createTranslator("zh"),
        "remote hub request failed",
      ),
    ).toBe("操作失败，请稍后重试。");
    expect(localizeAPIError({ status: 500, message: "服务暂时不可用" }, createTranslator("zh"))).toBe("服务暂时不可用");
  });

  it("localizes unavailable Docker errors without exposing platform diagnostics", () => {
    const error = {
      status: 503,
      code: "docker_unavailable",
      message: "open //./pipe/dockerDesktopLinuxEngine: The system cannot find the file specified",
    };

    expect(localizeAPIError(error, createTranslator("zh"))).toBe(
      "Docker 未启动或无法连接，请先启动 Docker 服务后重试。",
    );
    expect(localizeAPIError(error, createTranslator("en"))).toBe(
      "Docker is not running or cannot be reached. Start Docker and try again.",
    );
  });

  it("localizes the unfinished-room-task deletion conflict", () => {
    const error = {
      status: 409,
      code: "room_has_active_tasks",
      message: "finish or stop active room tasks before deleting the room",
    };

    expect(localizeAPIError(error, createTranslator("zh"))).toBe(
      "当前房间仍有未完成的任务。请先停止或完成任务，再归档或删除。",
    );
    expect(localizeAPIError(error, createTranslator("en"))).toBe(
      "This room still has unfinished tasks. Stop or complete them before archiving or deleting it.",
    );
  });

  it("localizes agent cleanup errors without exposing local paths", () => {
    const error = {
      status: 409,
      code: "agent_home_cleanup_failed",
      message: "unlinkat /Users/example/.csgclaw/agents/agent-1: directory not empty",
    };

    expect(localizeAPIError(error, createTranslator("zh"))).toBe(
      "智能体运行文件仍被占用。请启动 Docker 并等待其运行正常后重新删除；若 Docker 已启动，请重启 CSGClaw 后再试。",
    );
    expect(localizeAPIError(error, createTranslator("en"))).not.toContain("/Users/example");
  });
});

describe("template publishing diagnostics", () => {
  it.each(["zh", "en"] as const)("retains remote diagnostic context in %s", (locale) => {
    const message = "create remote hub template: remote hub request failed with status 500: repository unavailable";
    const formatted = localizeTemplatePublishError(
      { status: 502, code: "template_publish_failed", message },
      createTranslator(locale),
    );
    expect(formatted).toContain(message);
    expect(formatted).toContain("HTTP 502 · template_publish_failed");
    if (locale === "zh") expect(formatted).toContain("模板发布失败，请稍后重试。");
    expect(formatted.split(message)).toHaveLength(2);
  });
  it("keeps details for translated remote error codes", () => {
    expect(
      localizeTemplatePublishError(
        { code: "SYS-ERR-4", message: "template already exists: alice/guide" },
        createTranslator("zh"),
      ),
    ).toContain("template already exists: alice/guide");
  });
  it("handles missing diagnostics", () => {
    expect(localizeTemplatePublishError(null, createTranslator("zh"), "发布失败")).toBe("发布失败");
    expect(localizeTemplatePublishError(new Error("connection refused"), createTranslator("zh"))).toContain(
      "connection refused",
    );
  });
});

it("explains cloud cleanup retry exhaustion without exposing raw SQL diagnostics", () => {
  const error = {
    status: 504,
    code: "template_cloud_cleanup_pending",
    message: 'ERROR: duplicate key value violates unique constraint "idx_repositories_git_path"',
  };
  const result = localizeTemplatePublishError(error, createTranslator("zh"));
  expect(result).toContain("5 秒");
  expect(result).toContain("异步清理");
  expect(result).toContain("本地版本已保留");
  expect(result).not.toContain("SQL");
  expect(result).not.toContain("idx_repositories");
});
