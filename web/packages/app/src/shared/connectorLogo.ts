type ConnectorBrand = "feishu" | "github" | "gitlab";

function connectorBrand(value: string): ConnectorBrand | undefined {
  const name = value.trim().toLowerCase();
  if (name.startsWith("飞书") || /(?:^|[^a-z0-9])(?:feishu|lark)(?:$|[^a-z0-9])/u.test(name)) return "feishu";
  if (/(?:^|[^a-z0-9])github(?:$|[^a-z0-9])/u.test(name)) return "github";
  if (/(?:^|[^a-z0-9])gitlab(?:$|[^a-z0-9])/u.test(name)) return "gitlab";
  return undefined;
}

export function connectorLogoSource(name: string): string | undefined {
  const brand = connectorBrand(name);
  if (brand === "feishu") return "icons/feishu.png";
  if (brand === "github") return "icons/github.svg";
  if (brand === "gitlab") return "icons/gitlab.svg";
  return undefined;
}
