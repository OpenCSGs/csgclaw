# Manager Chat: a separate UI for the shared backend

A deliberately small, dependency-free frontend with OpenCSG sign-in, chat-model
selection, Manager conversations, and message synchronization. It uses the public
CSGClaw APIs, not the main React application's components or controller state.
The directory is already a static build; no npm install or bundler is needed.

```sh
# From the repository root; the original UI remains the default at /.
make build-ui UI=both
./bin/csgclaw-both serve --no-browser
# Open http://127.0.0.1:18080/ and http://127.0.0.1:18080/ui/custom/
# Use the port configured for your server if it differs.

# An archive with the same two UIs and the normal bundle layout:
make package-ui UI=both

# This example alone, at /:
make build-ui UI=custom

# A different frontend can still replace this example:
make build-ui UI=both UI_DIR=/path/to/another-ui/dist
```

## How it connects

- `index.html` and `style.css`: a compact light/dark chat layout, with mobile support.
- `api.mjs`: HTTP transport, deployment-relative API paths, room/model adapters.
- `app.mjs`: sign-in redirect, conversation selection/creation, model updates,
  message submission, SSE-triggered snapshots and periodic recovery.
- Authentication: `/api/v1/auth/status`, `/auth/login`, `/auth/logout`. Login
  returns to this UI, preserving the selected room. Authentication is owned by
  the shared server, so signing in/out affects both UIs. Other configured model
  providers can also be used without an OpenCSG account.
- Model catalog: `/api/v1/model-providers`. Model selection patches the current
  room's `configuration`, preserving its workspace and execution mode. It does
  not modify the Manager's global profile. It is also visible in the original UI.
- Conversations/messages: `/api/v1/bootstrap`, `/rooms`, `/messages`, `/events`.
  New chats are direct task sessions with the Manager user from bootstrap
  (`role: manager`); the agent profile is read using its stable `manager` name.
- Both mode strips `/ui/custom/` when resolving the backend base; any external
  gateway prefix is retained. Custom-only mode resolves from its own root.

Initialize the server and its Manager runtime/model services as usual. This UI
does not install runtimes or configure new providers. Provider configuration,
tools, attachments and rich interactive responses remain available in the full
UI. Messages here are displayed as safe plain text. There is no second backend
or agent runtime and no browser-side API-key storage.

Run the API/adapter regression tests with:

```sh
node --test scripts/manager-ui.test.mjs
```
