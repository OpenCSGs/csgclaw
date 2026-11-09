/** Parse CSS selectors and declarations without pretending jsdom computes browser layout. */
export function styleRules(source: string): CSSStyleRule[] {
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(source);
  function collect(rules: CSSRuleList): CSSStyleRule[] {
    return Array.from(rules).flatMap((rule) => {
      if (rule instanceof CSSStyleRule) return [rule];
      const nested = (rule as CSSGroupingRule).cssRules;
      return nested ? collect(nested) : [];
    });
  }
  return collect(sheet.cssRules);
}

/** Check every interaction state; compound :not() guards and structural selectors stay intact. */
export function skinRulesMatching(element: Element, rules: readonly CSSStyleRule[]): string[] {
  return rules
    .filter((rule) => {
      const properties = Array.from(rule.style);
      if (
        !properties.some((name) =>
          /^(color|background(?:-color)?|border(?:-(?:color|width|style))?|box-shadow|font(?:-size|-weight|-family)?)$/.test(
            name,
          ),
        )
      )
        return false;
      if (rule.selectorText.includes("::")) return false;
      const selector = rule.selectorText.replace(/:(hover|focus-visible|focus-within|focus|active)(?![\w-])/g, "");
      return element.matches(selector);
    })
    .map((rule) => rule.selectorText);
}
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

/** Follow relative stylesheet entrypoints in their declared order. */
export function stylesheetSource(file: string): string {
  return readFileSync(file, "utf8").replace(/@import\s+["']([^"']+)["'];/g, (_statement, relative: string) =>
    stylesheetSource(resolve(dirname(file), relative)),
  );
}
