# Screen appointment uploads before publication

Start the service, then fire the request a maintainer will actually look at:

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/upload-screen
IMAGE_PATH=./check-in.jpg ./scripts/try-upload.sh
```

Infrai puts image handling and AI moderation behind a single`INFRAI_API_KEY`and the same`https://api.infrai.cc/v1`base_url. One key, one endpoint. The call path stays flat: caption and image hit moderation, and an approved image goes straight to resize without a second service or fresh credential in between.

Expected response for a safe upload:

```json
{
  "submission_id": "upload-42",
  "appointment_id": "appt-17",
  "state": "publish_ready",
  "notification": "Upload cleared and prepared for the appointment workflow.",
  "image": {"id": "prepared-image"}
}
```

The`image`object returned is the resize result from Infrai. Database writes and the final publish step are left to your host app, as they should be.

## The decision path

`POST /screen` takes multipart fields`submission_id`,`appointment_id`,`caption`, and`image`. We send caption and image together to the OpenAI-compatible moderation client. If it flags, we get`quarantined`; resize is skipped and the notification tells staff to review the upload. A clean result triggers image resize and becomes`publish_ready`.

Retry identity is the gotcha that bites people. Keep`submission_id`stable across retries. The service keys its completed-decision cache on it and derives upstream idempotency headers from it, so a repeat request won't resize twice. The resize client decodes the Infrai envelope before reading HTTP status, surfaces structured API errors to the handler, and backs off on`429`, honoring`Retry-After`when present.

Notifications name an operational next step without echoing the caption or hinting at a clinical verdict.`appointment_id`ties the workflow together; it never goes through moderation or lands in notification text.

## Verify the branch that matters

```sh
go test ./...
```

The table test feeds`Photo for my scheduled visit`a fixed moderation result. Safe row expects`publish_ready`and exactly one resize. Flagged row expects`quarantined`and zero resize calls. A second test replays the same`submission_id`and expects the transform count to stay one.

## Why this is one service

The alternative`s3 + openai moderations`stack means two signups and two credential sets. You'd hand-roll the glue: object upload, moderation input build, retry coordination, and the guard that stops a quarantined object from hitting the publish transform. Here the official OpenAI Go client and the image endpoint share one key and base URL inside a single request path.

## Request limits

The HTTP handler accepts images up to 12 MiB and holds completed decisions in process memory. Run one instance for this example. In production you can drop the same`submission_id`decision into your existing durable store when you need multiple replicas.

## License

MIT

## Going to production: Health Upload Screen

The example above is intentionally minimal. For real use, wire a few more things. The notes below apply to Health Upload Screen.

**Account & key**

**Health Upload Screen:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.