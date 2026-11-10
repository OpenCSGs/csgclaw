import { readdirSync, readFileSync } from "node:fs";
import { posix, resolve } from "node:path";
import { readSourceDependencies, type SourceDependency } from "../helpers/sourceDependencies";

const dependencies = readSourceDependencies(resolve(process.cwd(), "src"));
const libraryDependencies = readSourceDependencies(resolve(process.cwd(), "../csgclaw-ui/src"));

function report(violations: SourceDependency[]): string {
  return violations.map(({ source, line, specifier }) => `${source}:${line} imports ${specifier}`).join("\n");
}

function pageOwner(file: string): string | undefined {
  const [directory, owner] = file.split("/");
  return directory === "pages" ? owner : undefined;
}

function isPrivatePageFile(file: string): boolean {
  const [, owner, ...parts] = file.split("/");
  if (!parts.length) return false;
  // A page's package entrypoint and named route component can be assembled by a shell.
  return parts.length !== 1 || !["index", owner].includes(posix.parse(parts[0]).name);
}

describe("shared UI and page ownership", () => {
  it("keeps the app UI entrypoint as a package re-export", () => {
    const entrypoint = resolve("src/components/ui");
    expect(readdirSync(entrypoint)).toEqual(["index.ts"]);
    expect(readFileSync(resolve(entrypoint, "index.ts"), "utf8").trim()).toBe('export * from "csgclaw-ui";');
  });

  it("keeps the UI package independent of application source", () => {
    const violations = libraryDependencies.filter(
      ({ specifier, target }) => specifier.startsWith("@/") || target === ".." || target.startsWith("../"),
    );
    expect(report(violations)).toBe("");
  });

  it("keeps shared business components and hooks independent of page-owned modules", () => {
    const violations = dependencies.filter(
      ({ source, target }) =>
        (source.startsWith("components/business/") || source.startsWith("hooks/")) && pageOwner(target),
    );
    expect(report(violations)).toBe("");
  });

  it("uses public entrypoints when composing another page", () => {
    const violations = dependencies.filter(
      ({ source, target }) =>
        pageOwner(source) && pageOwner(target) && pageOwner(source) !== pageOwner(target) && isPrivatePageFile(target),
    );
    expect(report(violations)).toBe("");
  });
});
