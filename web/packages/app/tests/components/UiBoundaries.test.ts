import { posix, resolve } from "node:path";
import { readSourceDependencies, type SourceDependency } from "../helpers/sourceDependencies";

const dependencies = readSourceDependencies(resolve(process.cwd(), "src"));

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
  it("keeps UI primitives independent of application data and controllers", () => {
    const violations = dependencies.filter(
      ({ source, target }) =>
        source.startsWith("components/ui/") &&
        /^(?:api|hooks|pages|shared\/(?:storage|realtime|platform))\//.test(target),
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
