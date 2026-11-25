# User feedback

The workspace user menu offers **Submit feedback**. Settings → Feedback & help opens the same form and also provides the GitHub link. Submitting through OpenCSG requires an active login. Signing in resumes the form.

The form accepts text, up to six JPG/JPEG/PNG images of at most 5 MiB each, and up to three ZIP/RAR/7Z/TAR/GZ archives of at most 10 MiB each. Users can browse for files, paste screenshots, or drop a batch. Invalid files are reported individually while accepted files remain in the form. `components/business/Feedback` owns the shared draft and dialog. `src/api/feedback.ts` sends images and archives as separate multipart fields.

`POST /api/v1/feedback` validates the multipart request, reads current login credentials, and supplies the running version, installed channel, username, and active OpenCSG site. The handler validates image and archive contents. `internal/feedback.Client` uploads images through the site's `/internal_api/upload?max_size=30m` endpoint and archives through `/internal_api/upload_with_ext?max_size=30m` so archive URLs can retain their extension. Both requests use the multipart field `file`. Local validation enforces the per-file and total limits. TAR, GZ, and standard unencrypted ZIP integrity checks read up to 256 MiB of expanded data per archive. Encrypted ZIP entries and other ZIP compression methods receive structural checks. RAR validation checks block headers and packed sizes without decoding file contents. RAR archives with encrypted headers cannot be verified. The JSON request to `/api/v1/csgbot/user-feedback` through StarHub sends image URLs in `screenshot_urls` and archive links at the start of `problem_description` so unfinished Markdown in the user description cannot hide them. The portal's storage-proxy mode forwards the `user_token` cookie as a Bearer credential, so the client supplies the active account access token. Redirects are rejected to keep credentials on their intended hosts. Upstream error logs redact URL query strings and fragments from the captured response body.

CSGBot's existing feedback service creates the Issue. Optional `csgclaw` context contains `site_url`, `version`, and `channel` for title and label generation. Existing JSON callers keep their behavior. GitLab credentials and project selection remain in CSGBot; image storage uses the portal's existing upload configuration.

A successful response displays a submission confirmation. Failures retain text and selected attachments. The browser prevents simultaneous submits and allows an explicit retry. An uncertain network outcome can result in a duplicate Issue if the user retries. Changing the active account or site and reloading the page clear the draft. GitHub feedback remains available without OpenCSG authentication.

Deploy the CSGBot optional context support before releasing this client so the CSGClaw title and label rules are applied. StarHub uses its existing authenticated CSGBot proxy. Real-site upload and submission integration remains to be verified.

## Screenshots

Browser captures of the actual Settings and FeedbackDialog components with a demo signed-in account. These previews show the UI and local image selection.

![Settings feedback actions](assets/settings-feedback.png)

![Feedback dialog with one selected image](assets/feedback-dialog.png)
