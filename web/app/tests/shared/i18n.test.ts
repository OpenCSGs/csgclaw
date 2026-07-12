import { createTranslator, localizeTemplateSourceTag } from "@/shared/i18n";

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

  it("localizes personal Hub source tags", () => {
    expect(localizeTemplateSourceTag("personal", "zh")).toBe("个人");
    expect(localizeTemplateSourceTag("personal", "en")).toBe("personal");
  });
});
