export function WorkspaceTopBar() {
  return (
    <header className="workspace-topbar">
      <div className="workspace-topbar-brand" aria-label="CSGClaw">
        <img
          className="workspace-topbar-logo workspace-topbar-logo-light"
          src="/brand/csgclaw-logo-light.svg"
          alt=""
          aria-hidden="true"
        />
        <img
          className="workspace-topbar-logo workspace-topbar-logo-dark"
          src="/brand/csgclaw-logo-dark.svg"
          alt=""
          aria-hidden="true"
        />
      </div>
    </header>
  );
}
