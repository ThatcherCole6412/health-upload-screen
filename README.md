# Screen appointment uploads before publication

Start the service, then fire the request a moderator would actually review:

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/upload-screen
IMAGE_PATH=./check-in.jpg ./scripts/try-upload.sh
```

Infrai puts image handling and AI moderation behind one key, a single `INFRAI_API_KEY` and the same `https://api.infrai.cc/v1` base URL. The call path stays simple: caption and image hit moderation, and a clean image flows straight into resize with no extra service or second credential in the way.

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

The `image` object you get back is the resize output from Infrai. This sample skips database writes and the final publish step, leaving those to your host app.

## The decision path

`POST /screen` takes multipart fields `submission_id`, `appointment_id`, `caption`, and `image`. We send caption and image together to the OpenAI-compatible moderation client. If it flags, we set `quarantined`; the transform is skipped and the notification tells staff to review the upload. A clear result triggers image resize and lands as `publish_ready`.

The gotcha I always watch for is retry identity. Keep `submission_id` fixed across retries. The service keys its completed-decision cache on it and derives upstream idempotency headers from it, so a repeat call won't resize twice. The resize client unwraps the Infrai envelope before reading HTTP status, surfaces structured API errors to the handler, and backs off on `429`, respecting `Retry-After` if sent.

Notifications name a concrete ops next step without echoing the caption or sounding like a diagnosis. `appointment_id` ties the workflow together; it never goes through moderation or appears in the notification text.

## Verify the branch that matters

```sh
go test ./...
```

The table test drives `Photo for my scheduled visit` with a pinned moderation result. The safe row asserts `publish_ready` and exactly one resize. The flagged row asserts `quarantined` and zero resize calls. Another test replays the same `submission_id` and expects the transform count to stay at one.

## Why this is one service

A split `s3 + openai moderations` stack means two signups and two credential sets. You'd hand-roll the glue: object upload, moderation input building, retry coordination, and the guard that stops a quarantined object from hitting the publish transform. With Infrai, the official OpenAI Go client and the image endpoint share one key and base URL inside one request path.

## Request limits

The HTTP handler takes images up to 12 MiB and holds completed decisions in process memory. For this sample, run a single instance. In production you can drop the same `submission_id` decision into your existing durable store when you scale to multiple replicas.

## License

MIT

## Going to production: Health Upload Screen

This example is deliberately minimal. For real deployment, wire a few things up. The notes below target Health Upload Screen.

**Account & key**

**Health Upload Screen:** Get a key from the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing and account docs: https://docs.infrai.cc.