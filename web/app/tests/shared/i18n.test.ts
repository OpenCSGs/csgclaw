import { createTranslator, localizeAPIError, localizeTemplateSourceTag } from "@/shared/i18n";

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
        { status: 409, code: "SPACE_ERR_1", message: "The space name already exists." },
        createTranslator("en"),
      ),
    ).toContain("same name already exists");
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
