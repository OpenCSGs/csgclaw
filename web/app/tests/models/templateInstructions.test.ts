import { extractManagedAgentInstructions, replaceManagedAgentInstructions } from "@/models/templateInstructions";

const start = "<!-- BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit) -->";
const end = "<!-- END CSGCLAW-INSTRUCTIONS -->";

it.each(["\n", "\r\n"])("preserves runtime sections and surrounding text (%j)", (newline) => {
  const document = [
    "User preface",
    start,
    "",
    "# Agent Instructions",
    "",
    "Original rules.",
    "",
    "# CSGClaw Runtime Boundary",
    "Boundary rules.",
    "",
    "# Managed Runtime Instructions",
    "Runtime rules.",
    end,
    "User suffix",
  ].join(newline);
  expect(extractManagedAgentInstructions(document)).toBe("Original rules.");
  const changed = replaceManagedAgentInstructions(document, "Updated rules.");
  expect(extractManagedAgentInstructions(changed)).toBe("Updated rules.");
  expect(changed).toContain(
    [
      "# CSGClaw Runtime Boundary",
      "Boundary rules.",
      "",
      "# Managed Runtime Instructions",
      "Runtime rules.",
      end,
      "User suffix",
    ].join(newline),
  );
  expect(changed.startsWith("User preface" + newline + start)).toBe(true);
  const cleared = replaceManagedAgentInstructions(document, "");
  expect(extractManagedAgentInstructions(cleared)).toBe("");
  expect(cleared).toContain("Boundary rules.");
  expect(cleared).not.toContain("Original rules.");
  expect(replaceManagedAgentInstructions(document, " Original rules.\n ")).toBe(document);
});

it("adds custom instructions without removing runtime-only blocks", () => {
  const document = `${start}\n\n# Managed Runtime Instructions\n\nKeep these rules.\n\n${end}\n`;
  const changed = replaceManagedAgentInstructions(document, "New custom rules.");
  expect(extractManagedAgentInstructions(changed)).toBe("New custom rules.");
  expect(changed).toContain("# Managed Runtime Instructions\n\nKeep these rules.");
});

it("does not rewrite documents for an empty or unchanged custom value", () => {
  const document = "# Existing unmanaged instructions\n";
  expect(replaceManagedAgentInstructions(document, "  \n")).toBe(document);
  const changed = replaceManagedAgentInstructions(document, "New rules.");
  expect(changed).toContain(document.trim());
  expect(extractManagedAgentInstructions(changed)).toBe("New rules.");
});
