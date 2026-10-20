# Room collaboration interface

This frontend design implements [requirement 3957](https://git-devops.opencsg.com/product/agentichub/requirements/-/work_items/3957), the [product design document](https://opencsg-china.feishu.cn/wiki/PqFbwZW92i4h0gkzUUHcLp5InMh), and the [task workspace prototype](http://124.221.45.243:8082/#/).

## Navigation and creation

`TaskWorkspaceSidebar` provides New task, Agents, Scheduled tasks, and the searchable conversation history throughout the workspace. Agent, team, and external-message detail routes retain this navigation. Existing deep links and settings query parameters keep their route behavior.

Creation actions open `/new`. `StartTaskPage` and `TargetPicker` handle the agent or team choice, the Manager-only team, the initial message, and empty-room creation. The picker also exposes the Manager-assisted agent and team creation actions. The conversation directory filter continues to use existing workspace-directory metadata.

Selecting a destination at viewport widths of 720px or less collapses the navigation so the destination is visible. Desktop navigation remains expanded when selecting a destination. Active conversations and navigation items expose `aria-current` and visible keyboard focus.

## Room tools

The on-demand collaboration room header presents these actions in order:

1. Member management.
2. Room tasks.
3. Activity.
4. Files.
5. Browser.

Room tasks use the existing tabbed `ChatSidePanel`. Its initial width is 496px; users can resize or maximize it, and saved widths remain respected. The task board uses three columns across two rows at this width, adapts to narrower containers, and can use six columns when maximized on a sufficiently wide screen. Each status column scrolls independently.

The task board and existing task page share `TaskBoard`, task grouping, worker labels, and status rendering. `RoomTaskPanel` scopes data through `roomTaskGroups`: only tasks whose `assignment_type` is `room` and whose `assignment_id` matches the current room are included. Parent cards retain their child-task progress. Selecting a card opens the existing `RoomTaskDialog` through `?task=<id>` and restores focus to the card when closed. Loading, refresh, errors, and empty columns remain visible states.

Completed room tasks with `goal_outcome=issues` display an "Ended with issues" marker on the card. `RoomTaskPanel` uses `roomTaskState` to supply this marker through `TaskBoard`'s optional status slot. The marker wraps within narrow cards and uses semantic warning colors in both themes.

Member management uses a separate 420px panel with current members, selectable invite candidates, external-message sources. The content scrolls while the cancel/invite controls stay visible. Below 780px of available conversation width, the panel covers the conversation and makes the covered content inert.

Selecting a room tool closes member management. Opening member management suspends the tabbed panel's display while preserving its tabs and browser drafts. Toolbar pressed states follow the visible panel. Message-selected threads retain priority during panel switching, and running work keeps an accessible stop action while its thread is hidden.

## Data flow and ownership

| Owner                                                  | Responsibility                                                                             |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------ |
| `WorkspaceLayout`, `TaskWorkspaceSidebar`              | Workspace layout and destination selection.                                                |
| `StartTaskPage`, `TargetPicker`                        | Creation workflow and target choice.                                                       |
| `useWorkspaceController`                               | Compose navigation, profile previews, and external-message settings callbacks.             |
| `useConversationController`                            | Member selection, membership mutations, pending state, errors, and room-switch protection. |
| `ConversationWorkspace`                                | Compose the header, conversation, member panel, task panel, and existing task details.     |
| `ChatSidePanel`, `useChatTools`                        | Persisted tabs, resizing, visibility, and tool activation.                                 |
| `RoomMembersPanel`                                     | Render member choices and source switches through callbacks.                               |
| `TaskBoard`, `RoomTaskPanel`                           | Render shared task cards and the room-scoped board.                                        |
| `models/roomTasks`, `models/tasks`, `models/taskBoard` | Pure filtering, grouping, and display helpers.                                             |
| `api/im`, existing task APIs                           | Request payloads, endpoint paths, and transport errors.                                    |

Member invitations and external-source connections reuse `POST /api/v1/rooms/invite`. Member removal and source disconnection reuse `DELETE /api/v1/rooms/{room_id}/members/{member_id}`. Mutations update the workspace bootstrap once, disable repeated submissions while pending, and retain failed choices for retry. A response for a previously selected room updates that room without closing the current room's panel or displaying its errors there.

The external-message management link opens `?dialog=settings&section=external`. `ExternalMessagesSection` reuses the existing notifier editor and refreshes both bootstrap and agents after changes. Closing settings preserves the room and member panel and restores keyboard focus to the opening control.

Business components consume callbacks and models; shared components and controllers keep their imports independent of page-private modules. New component styling uses CSS Modules and existing semantic tokens.

## Verification

Regression coverage includes shared card rendering, room filtering and child progress, completed room tasks with unresolved issues in both languages, tab persistence, member and tool switching, browser draft retention, thread selection, running-thread stop controls, membership errors and pending requests, settings focus restoration, and desktop/mobile navigation.

Browser checks use the production React application with local API fixtures. They cover 1280px and 1440px desktop layouts, a 390px viewport, light and dark themes, task details and focus return, panel maximization, membership changes and failed-request retry, external settings, and conversation switching. These checks validate frontend behavior with controlled responses; deployment-specific server integration requires an available CSGClaw backend.

Run the repository frontend checks with `pnpm --dir web/app check`. Validate production bundling with `pnpm --dir web/app exec vite build --outDir /tmp/csgclaw-room-collaboration-build --emptyOutDir`.

## Screenshots

These captures use demo API responses in the local application. Desktop captures use a 1440px viewport; mobile captures use a 390px viewport.

| View                                   | Preview                                                                                                                                                                                         |
| -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| New task and target selection          | [Light theme](screenshots/room-collaboration/new-task-light.png)                                                                                                                                |
| Room task board with unresolved issues | [Light theme](screenshots/room-collaboration/tasks-light.png), [maximized dark theme](screenshots/room-collaboration/tasks-dark.png), [mobile](screenshots/room-collaboration/tasks-mobile.png) |
| Member management and external sources | [Light theme](screenshots/room-collaboration/members-light.png), [mobile](screenshots/room-collaboration/members-mobile.png)                                                                    |
