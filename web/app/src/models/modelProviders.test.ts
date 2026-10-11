import { describe, expect, it } from "vitest";
import {
  mergeModelProviderModelIDs,
  modelProviderMessageLabel,
  modelProviderStatusLabel,
  type ModelProvider,
} from "./modelProviders";
import { createTranslator } from "@/shared/i18n";

describe("mergeModelProviderModelIDs", () => {
  it("includes video-only models and keeps generative models after chat models", () => {
    expect(
      mergeModelProviderModelIDs(
        ["chat-model", "shared-model"],
        ["image-model", "shared-model"],
        ["video-model", "shared-model"],
      ),
    ).toEqual(["chat-model", "shared-model", "image-model", "video-model"]);
  });
});

describe("model provider labels", () => {
  const zh = createTranslator("zh");
  const provider = { builtin: false, id: "default" } as ModelProvider;

  it("localizes provider statuses", () => {
    expect(modelProviderStatusLabel("connected", provider, zh)).toBe("已连接");
    expect(modelProviderStatusLabel("failed", provider, zh)).toBe("连接失败");
    expect(modelProviderStatusLabel("unknown", provider, zh)).toBe("尚未检查");
  });

  it("localizes known provider messages", () => {
    expect(modelProviderMessageLabel({ message: "connected", status: "connected" }, zh, "zh")).toBe("已连接");
    expect(modelProviderMessageLabel({ message: "base URL is required", status: "failed" }, zh, "zh")).toBe(
      "接口地址不能为空。",
    );
    expect(
      modelProviderMessageLabel(
        {
          message: "Claude Code auth was not found in macOS Keychain. Run csgclaw model auth login claude-code.",
          status: "failed",
        },
        zh,
        "zh",
      ),
    ).toBe("Claude Code 尚未登录。请运行 csgclaw model auth login claude-code。");
  });

  it("hides unknown English diagnostics only in Chinese", () => {
    const failed = { message: "HTTP 401 Unauthorized (/v1/models)", status: "failed" };
    expect(modelProviderMessageLabel(failed, zh, "zh")).toBe("检查失败，请核对连接信息后重试。");
    expect(modelProviderMessageLabel(failed, createTranslator("en"), "en")).toBe(failed.message);
  });
});
