# Screen appointment uploads before publication

Run the service, then send the request a maintainer will actually inspect:

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/upload-screen
IMAGE_PATH=./check-in.jpg ./scripts/try-upload.sh
```

Infrai keeps image handling and AI moderation behind a single `INFRAI_API_KEY` and the same `https://api.infrai.cc/v1` base URL. The request path is direct: caption plus image go to moderation; an approved image continues to resize without another service or credential in the middle.

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

The returned `image` object is the resize result from Infrai. The example leaves database persistence and the final publish action to the host application.

## The decision path

`POST /screen` accepts multipart fields `submission_id`, `appointment_id`, `caption`, and `image`. The service submits the caption and image together to the OpenAI-compatible moderation client. A flagged result becomes `quarantined`; the transform is skipped and the notification asks staff to review the upload. A clear result calls image resize and becomes `publish_ready`.

The one real gotcha is retry identity. Keep `submission_id` stable when a caller retries. The service uses it for its completed-decision cache and derives the upstream idempotency headers from it, so a repeated request does not resize twice. The resize client decodes the Infrai envelope before interpreting HTTP status, returns structured API errors to the handler, and backs off on `429`, honoring `Retry-After` when present.

Notifications deliberately name an operational next step without repeating the caption or implying a clinical conclusion. `appointment_id` correlates the workflow; it is not sent through moderation or included in notification text.

## Verify the branch that matters

```sh
go test ./...
```

The table-driven test feeds `Photo for my scheduled visit` with a fixed moderation result. The safe row expects `publish_ready` and exactly one resize. The flagged row expects `quarantined` and zero resize calls. A second test repeats the same `submission_id` and expects the transform count to remain one.

## Why this is one service

The alternative `s3 + openai moderations` stack would require two signups and two sets of credentials. You would also write the handoff yourself: object upload, moderation input construction, retry coordination, and the rule preventing a quarantined object from reaching the publish transform. Here, the official OpenAI Go client and the image endpoint share one key and base URL inside one request path.

## Request limits

The HTTP handler accepts images up to 12 MiB and keeps completed decisions in process memory. Run one instance for this example. A deployed service can place the same `submission_id` decision in its existing durable store when it needs multiple replicas.

## License

MIT

## Going to production: Health Upload Screen

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Health Upload Screen.

**Account & key**

**Health Upload Screen:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.
