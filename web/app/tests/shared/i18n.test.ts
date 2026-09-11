import { createTranslator, localizeAPIError, localizeTemplateSourceTag } from "@/shared/i18n";
import { messages } from "@/shared/i18n/messages";

describe("i18n messages", () => {
  it("keeps the human profile subtitle concise", () => {
    expect(createTranslator("en")("humanDetailSubtitle")).toBe("How you appear in chats, mentions, and collaboration.");
    expect(createTranslator("zh")("humanDetailSubtitle")).toBe("你在聊天、提及和协作中的显示方式。");
  });

  it("localizes connector controls instead of exposing translation keys", () => {
    const connectorLabels = {
      en: ["Manage connectors", "Connected", "Manage", "Disconnect"],
      zh: ["管理连接器", "已连接", "管理", "断开"],
    } as const;

    for (const locale of ["en", "zh"] as const) {
      const t = createTranslator(locale);
      expect([
        t("connectorManagerTitle"),
        t("connectorConnected"),
        t("connectorManage"),
        t("connectorDisconnect"),
      ]).toEqual(connectorLabels[locale]);
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

  it("preserves the upstream reason for generic template publishing failures", () => {
    const message =
      'publish hub template to "official": remote hub request failed with status 500: failed to update repository path';
    expect(localizeAPIError({ status: 400, code: "template_publish_failed", message }, createTranslator("zh"))).toBe(
      message,
    );
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
      "当前房间仍有未完成的任务。请先停止或完成任务，再删除房间。",
    );
    expect(localizeAPIError(error, createTranslator("en"))).toBe(
      "This room still has unfinished tasks. Stop or complete them before deleting the room.",
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
