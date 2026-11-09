/* eslint-disable react-refresh/only-export-components */
import { TooltipProvider } from "@/components/ui";
import type { ReactElement, ReactNode } from "react";
import {
  render as baseRender,
  renderHook as baseRenderHook,
  type RenderHookOptions,
  type RenderHookResult,
  type RenderOptions,
} from "@testing-library/react/dist/index.js";
export * from "@testing-library/react/dist/index.js";

function TooltipTestProvider({ children }: { children: ReactNode }) {
  return <TooltipProvider>{children}</TooltipProvider>;
}

export function render(ui: ReactElement, options: RenderOptions = {}) {
  const ProvidedWrapper = options.wrapper;
  const wrapper = ProvidedWrapper
    ? function Wrapper({ children }: { children: ReactNode }) {
        return (
          <TooltipTestProvider>
            <ProvidedWrapper>{children}</ProvidedWrapper>
          </TooltipTestProvider>
        );
      }
    : TooltipTestProvider;

  return baseRender(ui, { ...options, wrapper });
}

export function renderHook<Result, Props>(
  renderCallback: (initialProps: Props) => Result,
  options: RenderHookOptions<Props> = {},
): RenderHookResult<Result, Props> {
  const ProvidedWrapper = options.wrapper;
  const wrapper = ProvidedWrapper
    ? function Wrapper({ children }: { children: ReactNode }) {
        return (
          <TooltipTestProvider>
            <ProvidedWrapper>{children}</ProvidedWrapper>
          </TooltipTestProvider>
        );
      }
    : TooltipTestProvider;

  return baseRenderHook(renderCallback, { ...options, wrapper });
}
