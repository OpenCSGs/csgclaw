/* global document, localStorage, window */

(() => {
  const base = document.createElement("base");
  const pathname = window.location.pathname || "/";
  base.href = pathname.endsWith("/") ? pathname : `${pathname}/`;
  document.head.appendChild(base);

  try {
    const theme = localStorage.getItem("csgclaw.im.theme");
    const next = theme === "light" || theme === "dark" ? theme : "dark";
    document.documentElement.dataset.theme = next;
    document.documentElement.style.colorScheme = next;
  } catch {
    document.documentElement.dataset.theme = "dark";
    document.documentElement.style.colorScheme = "dark";
  }
})();
