# User feedback

The workspace user menu offers **Submit feedback**. Settings → Feedback & help opens the same form and also provides the GitHub link. The feedback sign-in dialog explains the account requirement, offers the GitHub option, and returns to the form after authorization.

The form accepts text, up to six JPG/JPEG/PNG images of at most 5 MiB each, and up to three ZIP/RAR/7Z/TAR/GZ archives of at most 10 MiB each. Users can browse for files, paste screenshots, or drop a batch. The form gives a reason for each rejected file and explains where diagnostic archives are uploaded and how their links are used. `components/business/Feedback` owns the shared draft and dialog. `src/api/feedback.ts` sends images and archives as separate multipart fields.

`POST /api/v1/feedback` validates the multipart request, reads current login credentials, and supplies the running version, installed channel, username, and active OpenCSG site. The handler validates image and archive contents. `internal/feedback.Client` uploads images through the site's `/internal_api/upload?max_size=30m` endpoint and archives through `/internal_api/upload_with_ext?max_size=30m` so archive URLs can retain their extension. Both requests use the multipart field `file`. Local validation enforces the per-file and total limits. TAR, GZ, and standard unencrypted ZIP integrity checks read up to 256 MiB of expanded data per archive. Encrypted ZIP entries and other ZIP compression methods receive structural checks. RAR validation checks block headers and packed sizes without decoding file contents. RAR archives with encrypted headers cannot be verified. The JSON request to `/api/v1/csgbot/user-feedback` through StarHub sends image URLs in `screenshot_urls` and archive links at the start of `problem_description` so unfinished Markdown in the user description cannot hide them. The portal's storage-proxy mode forwards the `user_token` cookie as a Bearer credential, so the client supplies the active account access token. Redirects are rejected to keep credentials on their intended hosts. Upstream error logs redact URL query strings and fragments from the captured response body.

CSGBot's existing feedback service creates the Issue. Optional `csgclaw` context contains `site_url`, `version`, and `channel` for title and label generation. Existing JSON callers keep their behavior. GitLab credentials and project selection remain in CSGBot; image storage uses the portal's existing upload configuration.

A successful response displays a submission confirmation with an issue number and link when the service supplies them. The Web UI stores draft text, files, and the submission reference in browser IndexedDB, scoped to the current account and OpenCSG site. It restores the matching draft after reload and clears it after confirmed success or explicit discard. If browser storage is unavailable, the form warns before leaving with unsaved work.

The feedback form sends a random `request_id`. The server reserves it by account and site before contacting the upstream service and records the confirmed receipt in `feedback-submissions.json` in the application data directory. `POST /api/v1/feedback/status` reads that local record. A confirmed reference returns the existing receipt. A definite rejection releases the reservation so the same reference can be retried. If the upstream outcome is ambiguous, the reservation remains and is never forwarded again. The status remains unknown when the upstream service fails to deliver a receipt. Legacy multipart clients may omit `request_id` and continue to use the original submission contract.

Deploy the CSGBot optional context support before releasing this client so the CSGClaw title and label rules are applied. StarHub uses its existing authenticated CSGBot proxy. Real-site upload and submission integration remains to be verified.

## Screenshots

Browser captures of the actual Settings and FeedbackDialog components with a demo signed-in account. These previews show the UI and local image selection.

![Settings feedback actions](assets/settings-feedback.png)

![Feedback dialog with one selected image](assets/feedback-dialog.png)
