// The script URL gives this fixture's root in custom-only and both builds.
// For a real frontend, configure its resource/router base and API base explicitly.
const appBase = new URL("./", import.meta.url);
const apiBase = new URL(appBase.pathname.endsWith("/ui/custom/") ? "../../api/v1/" : "./api/v1/", appBase);
try {
  const version = await fetch(new URL("version", apiBase));
  if (!version.ok) throw new Error(`Backend version: ${version.status}`);
  document.querySelector("#result").textContent = JSON.stringify(await version.json(), null, 2);
} catch (error) {
  document.querySelector("#result").textContent = error.message;
}
