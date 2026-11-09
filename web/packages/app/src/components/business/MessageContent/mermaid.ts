type MermaidAPI = {
  initialize: (options: Record<string, unknown>) => void;
  run: (options: { nodes: HTMLElement[] }) => Promise<void>;
};

let mermaidTheme = "dark";
let mermaidInstance: MermaidAPI | null = null;
let mermaidPromise: Promise<MermaidAPI> | null = null;

function configureMermaid(instance: MermaidAPI): void {
  instance.initialize({
    securityLevel: "strict",
    startOnLoad: false,
    theme: mermaidTheme === "dark" ? "dark" : "neutral",
  });
}

export function initializeMermaidTheme(theme = "dark"): void {
  mermaidTheme = theme;
  if (mermaidInstance) {
    configureMermaid(mermaidInstance);
  }
}

function loadMermaid(): Promise<MermaidAPI> {
  if (mermaidInstance) {
    return Promise.resolve(mermaidInstance);
  }
  if (!mermaidPromise) {
    mermaidPromise = import("mermaid").then((module) => {
      mermaidInstance = (module.default ?? module) as unknown as MermaidAPI;
      configureMermaid(mermaidInstance);
      return mermaidInstance;
    });
  }
  return mermaidPromise;
}

export function prepareMermaidBlocks(container: HTMLElement): HTMLElement[] {
  const mermaidBlocks = container.querySelectorAll("pre > code.language-mermaid");
  mermaidBlocks.forEach((code, index) => {
    const pre = code.parentElement;
    if (!pre || pre.dataset.enhanced === "true") {
      return;
    }
    const wrapper = document.createElement("div");
    wrapper.className = "mermaid";
    wrapper.textContent = code.textContent ?? "";
    wrapper.dataset.blockId = `${Date.now()}-${index}`;
    pre.replaceWith(wrapper);
  });
  return Array.from(container.querySelectorAll<HTMLElement>(".mermaid"));
}

export function renderMermaidBlocks(nodes: HTMLElement[]): Promise<void> | undefined {
  if (nodes.length === 0) {
    return undefined;
  }
  return loadMermaid().then((mermaid) => mermaid.run({ nodes }));
}
