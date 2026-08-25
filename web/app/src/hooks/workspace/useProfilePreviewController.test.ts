// @vitest-environment jsdom

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentLike } from "@/models/agents";
import type { IMUser, TranslateFn } from "@/models/conversations";
import { PROFILE_PREVIEW_OPEN_DELAY_MS, useProfilePreviewController } from "./useProfilePreviewController";

const t: TranslateFn = (key) => key;
const user: IMUser = { id: "user-agent", name: "Agent" };
const agent: AgentLike = { id: "agent", name: "Agent", user_id: user.id };

function setup() {
  const anchor = document.createElement("button");
  document.body.append(anchor);
  const closeConversationTools = vi.fn();
  const result = renderHook(() =>
    useProfilePreviewController({
      agentItems: [agent],
      closeConversationTools,
      openAgentDirectMessage: vi.fn().mockResolvedValue(undefined),
      selectAgent: vi.fn(),
      t,
      usersById: new Map([[user.id, user]]),
    }),
  );
  return { ...result, anchor, closeConversationTools };
}

describe("useProfilePreviewController", () => {
  afterEach(() => {
    document.body.replaceChildren();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("delays hover previews and cancels them when the pointer leaves", () => {
    vi.useFakeTimers();
    const { result, anchor } = setup();

    act(() => result.current.showParticipantPreview(user, anchor));
    act(() => vi.advanceTimersByTime(PROFILE_PREVIEW_OPEN_DELAY_MS - 1));
    expect(result.current.profilePreviewProps).toBeNull();

    act(() => result.current.scheduleProfilePreviewClose());
    act(() => vi.advanceTimersByTime(PROFILE_PREVIEW_OPEN_DELAY_MS));
    expect(result.current.profilePreviewProps).toBeNull();
  });

  it("opens clicked avatars immediately and closes the preview with Escape", () => {
    const { result, anchor, closeConversationTools } = setup();
    const focus = vi.spyOn(anchor, "focus");

    act(() => result.current.openParticipantPreview(user, anchor));
    expect(result.current.profilePreviewProps?.agent?.id).toBe(agent.id);
    expect(closeConversationTools).toHaveBeenCalledTimes(1);

    act(() => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" })));
    expect(result.current.profilePreviewProps).toBeNull();
    expect(focus).toHaveBeenCalledTimes(1);
  });
});
