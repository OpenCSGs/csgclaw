import { render, screen } from "@testing-library/react";
import { Select } from "@/components/ui";

describe("Select", () => {
  it("renders default portal content inside the app root", () => {
    const appRoot = document.createElement("div");
    appRoot.id = "root";
    document.body.appendChild(appRoot);

    try {
      render(
        <Select
          open
          value="remote"
          options={[
            { value: "remote", label: "remote" },
            { value: "codex", label: "Codex" },
          ]}
        />,
        { container: appRoot },
      );

      const content = document.querySelector(".csg-select-content");
      expect(content).toBeInTheDocument();
      expect(appRoot).toContainElement(content as HTMLElement);
      expect(screen.getByText("Codex")).toBeInTheDocument();
    } finally {
      appRoot.remove();
    }
  });

  it("allows callers to override the portal container", () => {
    const appRoot = document.createElement("div");
    appRoot.id = "root";
    const portalContainer = document.createElement("div");
    portalContainer.id = "select-portal";
    document.body.append(appRoot, portalContainer);

    try {
      render(
        <Select
          open
          value="remote"
          contentProps={{ portalContainer }}
          options={[
            { value: "remote", label: "remote" },
            { value: "codex", label: "Codex" },
          ]}
        />,
        { container: appRoot },
      );

      const content = document.querySelector(".csg-select-content");
      expect(content).toBeInTheDocument();
      expect(portalContainer).toContainElement(content as HTMLElement);
    } finally {
      appRoot.remove();
      portalContainer.remove();
    }
  });
});
