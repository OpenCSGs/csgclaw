const assets = import.meta.glob<string>("./assets/*", { query: "?url", import: "default", eager: true });
export function applicationAsset(path: string) {
  const name = path.replace("ui-lab/figma/application/", "");
  return assets[`./assets/${name}`];
}
export function resolveAssetStyle(value: string) {
  return value.replace(/ui-lab\/figma\/application\/[^"')\s]+/g, (path) => applicationAsset(path));
}
