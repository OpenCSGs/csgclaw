package codex

const feishuLarkCLIManagedInstructions = `### Feishu lark-cli Access

- This worker is bound to a Feishu app through lark-cli. Plain ` + "`lark-cli ...`" + ` commands inherit the current worker context from ` + "`LARK_CHANNEL=1`" + `, ` + "`LARK_CHANNEL_HOME`" + `, ` + "`LARK_CHANNEL_PROFILE`" + `, ` + "`LARK_CHANNEL_CONFIG`" + `, and ` + "`LARKSUITE_CLI_CONFIG_DIR`" + `.
- Run every lark-cli command directly through ` + "`command_execution`" + `. Do not invoke lark-cli through ` + "`mcp_tool_call`" + `, MCP code execution, Node.js or Python subprocesses, or another tool wrapper; those environments may strip the worker's ` + "`LARK*`" + ` variables and read the host default profile instead.
- Treat a ` + "`not_configured`" + ` result from any non-` + "`command_execution`" + ` environment as invalid. Retry the same read-only status command once through ` + "`command_execution`" + ` before telling the user that lark-cli is not configured.
- Do not unset those variables, do not use the host default lark-cli profile, and do not read or print lark-cli config files, app secrets, access tokens, refresh tokens, OAuth device codes, or CSGClaw API tokens.
- If lark-cli reports that the lark-channel context is not bound, stop and tell the user to initialize lark-cli for this worker from the Feishu channel profile page or restart the worker after initialization. Do not run bind manually from an ordinary prompt.
- For Feishu Doc/Docx file tokens, first try ` + "`lark-cli docs +fetch --api-version v2 --doc <file_token> --doc-format markdown`" + `. If this lark-cli version does not support that exact syntax, inspect ` + "`lark-cli docs --help`" + ` once and use the equivalent current read-only command for the same token.
- For Feishu Drive/Wiki file nodes, use the current lark-cli drive download/read-only command and write downloaded files under the current workspace, for example ` + "`./downloads/`" + `. Do not upload generated local files back to Feishu unless the user explicitly asks and the available command is clearly write-capable and authorized.

### Feishu Historical Attachment Recovery

- Apply these rules only when the hidden channel context identifies the current request as Feishu and provides the current ` + "`chat_id`" + `. When the user refers to a previously uploaded Feishu file that is absent from the workspace, search only that current chat before asking for a re-upload.
- List message metadata without downloading resources first: ` + "`lark-cli im +chat-messages-list --as bot --chat-id <current_chat_id> --order desc --page-size 50 --no-reactions --format json`" + `. Use the user's time description with ` + "`--start`" + ` or ` + "`--end`" + ` when available, and follow pagination only as needed.
- Do not search, list, or inspect other chats. Do not use ` + "`--download-resources`" + ` during discovery because it downloads every eligible resource in the result set.
- Match candidates using ` + "`message_id`" + `, ` + "`msg_type`" + `, filename or resource marker in ` + "`content`" + `, surrounding message text, sender, and creation time. The selected resource key and ` + "`message_id`" + ` must come from the same message; never guess or combine identifiers.
- If exactly one candidate matches, download only that resource with ` + "`lark-cli im +messages-resources-download --as bot --message-id <message_id> --file-key <resource_key> --type <file_or_image> --output downloads/feishu/<safe_name>`" + `. Use ` + "`image`" + ` for image keys and ` + "`file`" + ` for files, audio, or video. Keep the output path relative and free of ` + "`..`" + ` traversal.
- If multiple candidates match, show a concise list without raw resource keys and ask the user to choose. If none match, state that no matching attachment was found in the current Feishu conversation.
- Keep Bot identity explicit with ` + "`--as bot`" + `. If Bot access fails because of missing scopes or chat membership, report the lark-cli error and tell the user which Bot permission or membership is required; do not silently retry as a user.
- If either shortcut is unavailable, inspect its ` + "`--help`" + ` once. If it is still unavailable, tell the user to upgrade lark-cli; do not install or upgrade lark-cli from the worker prompt.

- Start ` + "`lark-cli auth login`" + ` only in a Feishu private chat with the user who should own the authorization. In group chats, ask the user to open a private chat instead.
- Prefer the two-step OAuth flow when user authorization is needed: run ` + "`lark-cli auth login --no-wait --json --recommend`" + `, show the verification URL plainly to the user, then wait in the foreground with ` + "`lark-cli auth login --device-code <code>`" + `. Do not background the device-code wait.
- After user OAuth succeeds, silently converge identity policy with ` + "`lark-cli config strict-mode off`" + ` and ` + "`lark-cli config default-as auto`" + ` before retrying a user-identity read. Do not ask the user to choose those internal settings.`
