# Watcher Outbound Webhooks Integration Guide

Watcher can emit durable, outbound HTTP webhook events to external systems to announce watcher lifecycle updates, rollout completions, rollback statuses, and service health changes. 

This document serves as the integration guide and public API contract for webhook interactions in Watcher (`schema_version: "v1"`).

Companion references:

- In-app docs page: `/docs/webhooks`
- In-app webhook hub: `/webhooks`
- OpenAPI contract: [`web/static/webhooks.openapi.yaml`](../web/static/webhooks.openapi.yaml)

---

## 1. Webhook Delivery Model & Guarantees

Watcher's webhook engine is built with reliability in mind, providing the following guarantees and behaviors:

| Attribute | Guarantee & Behavior |
| :--- | :--- |
| **Delivery Mechanism** | Outbound `POST` requests sending `application/json` payload bodies. |
| **Delivery Guarantee** | **At-least-once** delivery. Due to network failures or retries, your endpoint may receive the same event multiple times. Receivers **must** deduplicate processing using the unique `event_id` field. |
| **Delivery Ordering** | **FIFO (First-In, First-Out) per Watcher**. Webhook events are processed sequentially in the order they were queued. If an event fails and is scheduled for a retry, subsequent events for that same watcher will wait behind it to preserve temporal order. |
| **Success Criteria** | Any HTTP response code in the `2xx` range (e.g., `200 OK`, `202 Accepted`) is treated as a successful delivery. |
| **Retry Behavior** | Network errors, timeouts, `429 Too Many Requests`, and server errors (`5xx`) trigger automatic retries. Retries are scheduled according to the `webhook_retry_schedule_sec` settings. |
| **Non-Retryable Errors** | Client errors in the `4xx` range (except `429`), such as `400 Bad Request`, `401 Unauthorized`, or `404 Not Found`, are classified as final failures. The event is moved directly to the `exhausted` state. |
| **Auto-Pause Circuit Breaker** | If enabled (`webhook_auto_pause_enabled`), reaching the threshold of consecutive failures will automatically pause outbound delivery for that watcher. New events generated while paused are kept as `suppressed` in the outbox. |
| **Manual Resume / Replays** | Resuming a paused outbox gives operators the option to replay suppressed events in their original chronological order, generating new `delivery_id` headers but preserving the original `event_id` and payload. |

---

## 2. HTTP Request Structure

When delivering an event, Watcher sends an HTTP request with the following details:

- **Method**: `POST`
- **Content-Type**: `application/json`
- **Headers**:
  - `webhook-id`: Stable event identifier used for idempotency across retries.
  - `webhook-timestamp`: Unix timestamp in seconds for the delivery attempt.
  - `webhook-signature`: Standard Webhooks HMAC-SHA256 signature over `webhook-id.webhook-timestamp.raw_body`.
  - `X-Watcher-Event`: The type of event being delivered (e.g. `watcher.deployment_succeeded`).
  - `X-Watcher-Delivery-ID`: A unique string identifying this specific delivery attempt (e.g. `dlv_01j0deployok_1`).

### 2.1 Shared Envelope Contract

Every webhook event includes the same top-level envelope before any event-specific nested object such as `version`, `attempt`, `service`, or `health`. Watcher now includes both the Standard Webhooks fields and its legacy convenience fields for easier migration.

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `type` | `string` | Standard Webhooks event type. This matches `event_type`. |
| `timestamp` | `string` (`RFC3339 date-time`) | Standard Webhooks event timestamp. This matches `occurred_at`. |
| `data` | `object` | Standard Webhooks event data envelope. Includes `event_id`, `watcher`, `summary`, and the event-specific nested object. |
| `schema_version` | `string` | Version of the webhook payload contract. For the current public contract this is `v1`. |
| `event_id` | `string` | Stable identifier for the business event itself. Use this for idempotency and deduplication across retries or replays. |
| `event_type` | `string` | Event name such as `watcher.version_found`. This matches the `X-Watcher-Event` header and tells you which nested object to expect. |
| `occurred_at` | `string` (`RFC3339 date-time`) | When Watcher created the event record, not when your endpoint received it. |
| `watcher.id` | `integer` | Internal database ID of the Watcher instance that emitted the event. It is stable inside that Watcher deployment and useful for correlating multiple events from the same watcher. |
| `watcher.name` | `string` | Human-facing watcher name configured in Watcher UI. Useful for logs, dashboards, and notifications. |
| `summary` | `string` | Human-readable description of the event. Safe for display or logs, but receivers should not automate business logic from this text. Use structured fields instead. |

### 2.2 `watcher.version_found` Contract

`watcher.version_found` is a discovery event. It means Watcher observed a newer remote version than the watcher's currently active version. It does **not** guarantee that deployment has started or will start.

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `version.discovered_version` | `string` | The newer remote version string that Watcher found during polling. This is the candidate release version that triggered the event. |
| `version.current_version` | `string` | The watcher's current active version at the time the event was emitted. Compare this with `discovered_version` to see what changed. |
| `version.will_deploy` | `boolean` | Whether Watcher intends to proceed with deployment for the discovered version after discovery. `true` means deployment is allowed to continue; `false` means discovery happened but rollout is currently blocked. |
| `version.block_reason` | `string` | Reason deployment will not proceed when `will_deploy` is `false`. Treat this as explanatory operator-facing text, not a fixed enum. Expect an empty string when there is no block. |

Receiver guidance for this event:

- Treat it as a notification that a new candidate exists.
- Do not assume a deployment attempt already exists.
- Use `will_deploy=false` plus `block_reason` to distinguish "new version found but no rollout will happen" from "new version found and rollout can proceed".
- Use `event_id` for deduplication, not `discovered_version` alone.

---

## 3. Webhook Receiver Examples

To integrate with Watcher, write an HTTP server endpoint that verifies the Standard Webhooks signature, logs/processes the payload asynchronously, and returns a `200 OK` or `202 Accepted` immediately.

### Go Example (Using Standard Library)

```go
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
)

const signingSecret = "whsec_your_configured_secret"

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Verify Request Method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. Read the exact request body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 3. Verify Standard Webhooks signature headers
	wh, err := standardwebhooks.NewWebhook(signingSecret)
	if err != nil {
		http.Error(w, "Invalid receiver secret", http.StatusInternalServerError)
		return
	}
	if err := wh.Verify(body, r.Header); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 4. Extract Watcher metadata headers
	eventType := r.Header.Get("X-Watcher-Event")
	deliveryID := r.Header.Get("X-Watcher-Delivery-ID")

	// 5. Parse JSON Payload Envelope
	var envelope struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		EventID   string `json:"event_id"`
		Summary   string `json:"summary"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 6. Log metadata and handoff to a background queue
	log.Printf("Received Webhook: ID=%s Type=%s (%s) DeliveryID=%s",
		envelope.EventID, envelope.Type, envelope.Summary, deliveryID)

	// TODO: Store eventID in database to deduplicate future deliveries
	// Go routine or message queue handoff here to process actual payload...

	// 7. Respond with 200 OK or 202 Accepted
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"received"}`))
}

func main() {
	http.HandleFunc("/watcher-webhooks", webhookHandler)
	log.Println("Webhook receiver running on :8090...")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
```

### Node.js Example (Express + TypeScript)

```typescript
import express, { Request, Response } from 'express';
import crypto from 'crypto';

const app = express();
app.use(express.json({
  verify: (req, _res, buf) => {
    (req as Request & { rawBody?: Buffer }).rawBody = Buffer.from(buf);
  }
}));

const SIGNING_SECRET = 'whsec_your_configured_secret';

type RawBodyRequest = Request & { rawBody?: Buffer };

interface WebhookEnvelope {
  type: string;
  timestamp: string;
  schema_version: string;
  event_id: string;
  event_type: string;
  occurred_at: string;
  watcher: {
    id: number;
    name: string;
  };
  summary: string;
}

function verifyStandardWebhookSignature(req: RawBodyRequest, secret: string): boolean {
  const msgId = req.header('webhook-id');
  const msgTimestamp = req.header('webhook-timestamp');
  const msgSignature = req.header('webhook-signature');
  const rawBody = req.rawBody;

  if (!msgId || !msgTimestamp || !msgSignature || !rawBody) {
    return false;
  }

  const unsignedSecret = secret.replace(/^whsec_/, '');
  const key = Buffer.from(unsignedSecret, 'base64');
  const signedContent = `${msgId}.${msgTimestamp}.${rawBody.toString('utf8')}`;
  const expected = crypto.createHmac('sha256', key).update(signedContent).digest('base64');

  return msgSignature
    .split(' ')
    .some((entry) => {
      const [version, signature] = entry.split(',');
      if (version !== 'v1' || !signature) return false;
      return crypto.timingSafeEqual(Buffer.from(signature), Buffer.from(expected));
    });
}

app.post('/watcher-webhooks', (req: RawBodyRequest, res: Response): any => {
  const eventType = req.headers['x-watcher-event'];
  const deliveryId = req.headers['x-watcher-delivery-id'];

  // 1. Verify Standard Webhooks signature with the raw body.
  if (!verifyStandardWebhookSignature(req, SIGNING_SECRET)) {
    return res.status(401).send('Unauthorized');
  }

  const payload = req.body as WebhookEnvelope;

  // 2. Validate basic structure
  if (!payload.event_id || !payload.type) {
    return res.status(400).send('Bad Request: Missing event_id or type');
  }

  console.log(`Processing event ${payload.event_id} of type ${payload.type}. Attempt delivery: ${deliveryId}`);
  console.log(`Summary: ${payload.summary}`);

  // TODO: Check if event_id has been processed recently (deduplication)
  // Process the webhook payload asynchronously (e.g., notify Slack, update status boards)

  // 3. Return Success Status Code
  res.status(200).json({ status: 'accepted' });
});

app.listen(8090, () => {
  console.log('Webhook server listening on port 8090');
});
```

---

### 3.1 Zero-to-Ship Setup Checklist

Use this checklist when wiring a new receiver from scratch:

1. **Choose the receiver endpoint**
   - Expose a `POST` endpoint reachable by the Watcher host.
   - Make sure your reverse proxy, firewall, and TLS settings allow Watcher to reach it.

2. **Generate a signing secret**
   - Generate a random Standard Webhooks HMAC secret in the `whsec_...` format.
   - Use one secret per logical endpoint or consumer whenever practical.
   - Store it in your receiver secret store first, then copy the same value into Watcher.

3. **Implement raw-body verification**
   - Read the exact request body bytes before re-serializing JSON.
   - Verify `webhook-id`, `webhook-timestamp`, and `webhook-signature` using the shared secret.
   - Reject requests with invalid signatures or timestamps outside your acceptable replay window.

4. **Add idempotency protection**
   - Deduplicate on `event_id`.
   - Treat `X-Watcher-Delivery-ID` as the transport-attempt identifier only.
   - Store recent processed event IDs in a durable store such as Redis or your primary database.

5. **Configure Watcher**
   - Set a global default webhook URL and signing secret, or configure watcher-specific overrides.
   - Enable webhook delivery explicitly on the watcher.
   - Select the event types you want emitted.

6. **Send a test webhook**
   - Use the Watcher UI action to queue `watcher.webhook_test`.
   - Confirm the receiver accepted the request and verified the signature.
   - Confirm Watcher delivery history shows the attempt as `succeeded`.

7. **Exercise failure handling before production**
   - Intentionally return a `401`, `400`, and `500` once each.
   - Verify Watcher treats them as expected:
     - `400` and `401` are terminal failures
     - `500` is retryable
     - repeated failures can auto-pause the watcher webhook

8. **Go live**
   - Turn on the real business events you want.
   - Monitor delivery history, exhausted events, and your receiver logs during the first rollout window.

### 3.2 Secret Management Guidance

Treat the signing secret like any other production credential:

- Generate high-entropy secrets only. Do not hand-write short or guessable values.
- Prefer one secret per endpoint, environment, or tenant boundary instead of reusing one secret everywhere.
- Store secrets in your normal secret-management system, not in source control.
- Never expose secrets in logs, screenshots, support messages, or dashboard payload views.
- When rotating a secret:
  1. add the new secret to the receiver
  2. update Watcher to send with the new secret
  3. verify delivery success
  4. remove the old secret from the receiver

**Current Watcher limitation:** this implementation signs with one active HMAC secret at a time and does not yet emit multiple concurrent signatures for zero-downtime secret rotation. Plan rotation carefully and validate with `watcher.webhook_test` immediately after changing the secret.

### 3.3 Receiver Implementation Notes

- **Verify before trusting payload contents.** Do not parse the request into domain actions before signature verification passes.
- **Use the raw request body.** Parsing JSON and serializing it again can change whitespace or field ordering and break verification.
- **Keep request handling fast.** Accept, verify, enqueue work internally, and return `2xx`. Do not block the webhook response on slow downstream business logic.
- **Log stable fields.** At minimum log `event_id`, `type`, `X-Watcher-Delivery-ID`, and the HTTP status you returned.
- **Enforce timestamp tolerance.** Standard Webhooks verification should reject requests too far in the past or future to reduce replay risk.

### 3.4 Migration Notes For Existing Watcher Consumers

If you already integrated with Watcher’s earlier bearer-token-oriented webhook contract, the migration path is:

1. Remove the old expectation that Watcher authenticates with `Authorization: Bearer ...`.
2. Add Standard Webhooks verification using the shared `whsec_...` secret.
3. Keep reading the existing Watcher event fields if you already depend on them.
4. Prefer the new standard top-level fields for new integrations:
   - `type`
   - `timestamp`
   - `data`
5. Continue deduplicating on `event_id`; that behavior did not change.

Watcher currently keeps both the Standard Webhooks envelope fields and the legacy Watcher convenience fields in the payload for compatibility during migration. New receivers should prefer the standard shape first and use the legacy fields only when they add clarity or ease transition.

### 3.5 Production Readiness Checklist

Before treating the integration as shipped, confirm all of the following:

- The receiver verifies signatures against the raw request body.
- The receiver enforces idempotency on `event_id`.
- The receiver tolerates out-of-order transport retries only by dedupe, not by assuming exactly-once delivery.
- The receiver returns `2xx` only after it has durably accepted the event for its own internal processing.
- The receiver logs enough metadata to investigate replay, retry, and exhausted delivery incidents.
- Your team knows how to resume paused webhooks and when to replay suppressed events.
- Your team knows which event types are enabled and which downstream automations depend on them.
- Secret rotation has been rehearsed in a non-production environment.

### 3.6 Common Ship-Time Failure Modes

- **Signature verifies locally but fails in production**
  - Usually caused by verifying a parsed JSON body instead of the raw bytes, or by a proxy/body parser consuming the raw stream incorrectly.

- **All requests fail with `401`**
  - Usually the receiver and Watcher do not share the same `whsec_...` secret, or the receiver’s clock tolerance is too strict.

- **Retries keep happening for the same business event**
  - The receiver may be doing real work before responding and timing out, or it may be returning `5xx` after partial processing.

- **Duplicate downstream work**
  - The receiver is using `X-Watcher-Delivery-ID` instead of `event_id` for idempotency.

- **Webhook traffic stops after a bad rollout**
  - The watcher likely auto-paused after repeated failures. Resume manually after fixing the receiver, then decide whether suppressed events should be replayed.

---

## 4. OpenAPI Specification Component Schemas

Below is the OpenAPI 3.0 YAML definition of the payload models. Use these schemas in your API gateway or mock servers to validate incoming webhook formats.

```yaml
components:
  schemas:
    BaseWebhookEvent:
      type: object
      required:
        - schema_version
        - event_id
        - event_type
        - occurred_at
        - watcher
        - summary
      properties:
        schema_version:
          type: string
          example: "v1"
          description: "Version of the webhook payload schema structure."
        event_id:
          type: string
          example: "evt_01j0versionfound"
          description: "Unique identifier for this event. Use this value to deduplicate event handling."
        event_type:
          type: string
          example: "watcher.version_found"
          description: "Matches the X-Watcher-Event header."
        occurred_at:
          type: string
          format: date-time
          example: "2026-06-17T02:15:10Z"
          description: "RFC3339 formatted timestamp representing when the event was generated."
        watcher:
          type: object
          required:
            - id
            - name
          properties:
            id:
              type: integer
              example: 12
              description: "Database ID of the originating RepoWatcher."
            name:
              type: string
              example: "api-prod"
              description: "Display name of the watcher."
        summary:
          type: string
          example: "Watcher api-prod found version v1.4.2"
          description: "Human-readable summary sentence describing the event."

    DeployAttempt:
      type: object
      required:
        - id
        - kind
        - reason
        - status
        - triggered_by
        - target_version
        - from_version
      properties:
        id:
          type: integer
          example: 301
          description: "ID of the deploy/rollback transaction log in Watcher."
        kind:
          type: string
          enum: [deploy, rollback]
          example: "deploy"
          description: "Determines whether this was a deployment or recovery rollback attempt."
        reason:
          type: string
          example: "new_version_found"
          description: "Indicates trigger source, e.g. new_version_found, manual_redeploy, auto_after_failed_deploy, manual_rollback."
        status:
          type: string
          enum: [succeeded, failed]
          example: "succeeded"
          description: "Terminal state of the attempt transaction."
        triggered_by:
          type: string
          enum: [agent, manual]
          example: "agent"
          description: "Who initiated the execution."
        target_version:
          type: string
          example: "v1.4.2"
          description: "Target version string of the release."
        from_version:
          type: string
          example: "v1.4.1"
          description: "Previous active version before starting the transaction."
        failed_target_version:
          type: string
          example: ""
          description: "Only populated for rollbacks: the version that failed and triggered the rollback."
        failure_phase:
          type: string
          example: ""
          description: "Identifies step of failure, e.g. download, extract, activate_release, start_services, health_check."
        error:
          type: string
          example: ""
          description: "Failure reason or terminal process error message."
        parent_attempt_id:
          type: integer
          nullable: true
          example: null
          description: "Link to deployment event that triggered an automatic rollback attempt."
        root_attempt_id:
          type: integer
          example: 301
          description: "Identifies the parent transaction chain ID."

    WatcherVersionFoundEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - version
          properties:
            version:
              type: object
              required:
                - discovered_version
                - current_version
                - will_deploy
                - block_reason
              properties:
                discovered_version:
                  type: string
                  example: "v1.4.2"
                current_version:
                  type: string
                  example: "v1.4.1"
                will_deploy:
                  type: boolean
                  example: false
                block_reason:
                  type: string
                  example: "watcher paused"

    WatcherDeploymentSucceededEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    WatcherDeploymentFailedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    WatcherRollbackSucceededEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    WatcherRollbackFailedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - attempt
          properties:
            attempt:
              $ref: '#/components/schemas/DeployAttempt'

    ServiceHealthChangedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - service
            - health
          properties:
            service:
              type: object
              required:
                - id
                - name
                - service_type
                - health_check_url
              properties:
                id:
                  type: integer
                  example: 87
                name:
                  type: string
                  example: "api-prod-web"
                service_type:
                  type: string
                  example: "nssm"
                health_check_url:
                  type: string
                  example: "https://api.example.com/health"
            health:
              type: object
              required:
                - previous_status
                - current_status
                - http_status
                - error
                - checked_at
                - source
              properties:
                previous_status:
                  type: string
                  example: "healthy"
                current_status:
                  type: string
                  example: "unhealthy"
                http_status:
                  type: integer
                  example: 503
                error:
                  type: string
                  example: "unexpected status code 503"
                checked_at:
                  type: string
                  format: date-time
                  example: "2026-06-17T03:02:11Z"
                source:
                  type: string
                  example: "manual"

    WatcherWebhookTestEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - triggered_by
          properties:
            triggered_by:
              type: string
              example: "manual"

    WebhookDeliveryExhaustedEvent:
      allOf:
        - $ref: '#/components/schemas/BaseWebhookEvent'
        - type: object
          required:
            - failed_delivery
          properties:
            failed_delivery:
              type: object
              required:
                - event_id
                - event_type
                - delivery_id
                - attempt_number
                - response_status_code
                - error
                - summary
              properties:
                event_id:
                  type: string
                  example: "evt_01j0deployfail"
                event_type:
                  type: string
                  example: "watcher.deployment_failed"
                delivery_id:
                  type: string
                  example: "dlv_01j0deployfail_4"
                attempt_number:
                  type: integer
                  example: 4
                response_status_code:
                  type: integer
                  example: 503
                error:
                  type: string
                  example: "server returned 503"
                summary:
                  type: string
                  example: "Deployment of api-prod to v1.4.3 failed during health_check"
```

---

## 5. Event Catalog & Payload Reference

This section details every event type fired by Watcher. Each block defines the trigger conditions, behaviors, and provides a matching JSON payload example.

### `watcher.version_found`

* **Trigger**: Fired when a polling cycle discovers a newer remote release tag than the watcher's current active version.
* **Behavior**:
  - Emitted exactly once per remote version discovered.
  - Dedicated deduplication is performed so subsequent poll checks for the same version do not repeat the event.
  - This event fires even if deployment is currently blocked (e.g., watcher is paused, release ref mismatch, or retry threshold exceeded). Check the nested `version.will_deploy` and `version.block_reason` fields to see if the watcher will proceed to roll out this version.
* **Field semantics**:
  - `version.discovered_version` is the newly discovered remote version.
  - `version.current_version` is the currently active watcher version when discovery happened.
  - `version.will_deploy=true` means Watcher can proceed toward deployment after discovery.
  - `version.will_deploy=false` means this is still a real discovery event, but rollout is blocked; `version.block_reason` explains why.
  - `summary` is for human reading only. Automation should use the structured fields above.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0versionfound",
  "event_type": "watcher.version_found",
  "occurred_at": "2026-06-17T02:15:10Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "version": {
    "discovered_version": "v1.4.2",
    "current_version": "v1.4.1",
    "will_deploy": false,
    "block_reason": "watcher paused"
  },
  "summary": "Watcher api-prod found version v1.4.2"
}
```

---

### `watcher.deployment_succeeded`

* **Trigger**: Fired when a deployment attempt finishes successfully (i.e. artifact is extracted, services are swapped, and health checks pass).
* **Behavior**:
  - Emitted only after the active version is updated in the database.
  - Useful for updating deployment dashboards or triggering post-deployment automation (e.g. cache purges).
* **Field contract**:

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `watcher.id` | `integer` | Internal ID of the watcher that emitted the event. |
| `watcher.name` | `string` | Human-facing watcher name. |
| `attempt.id` | `integer` | Unique ID of this deploy attempt record. |
| `attempt.kind` | `string` | Always `deploy` for this event. |
| `attempt.reason` | `string` | Why the deploy was started, such as `new_version_found` or `manual_redeploy`. |
| `attempt.triggered_by` | `string` | Who initiated the deploy flow, such as `agent` or `manual`. |
| `attempt.status` | `string` | Always `succeeded` for this event. |
| `attempt.target_version` | `string` | Version that was successfully deployed. |
| `attempt.from_version` | `string` | Version that was active before the deploy started. |
| `summary` | `string` | Human-readable summary for logs and UI. Do not automate from this text. |
* **Interpretation**:
  - This event means the deploy reached terminal success.
  - If health checks are part of the deploy flow, success is emitted only after they pass.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0deployok",
  "event_type": "watcher.deployment_succeeded",
  "occurred_at": "2026-06-17T02:20:03Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 301,
    "kind": "deploy",
    "reason": "new_version_found",
    "status": "succeeded",
    "triggered_by": "agent",
    "target_version": "v1.4.2",
    "from_version": "v1.4.1",
    "failed_target_version": "",
    "failure_phase": "",
    "error": "",
    "parent_attempt_id": null,
    "root_attempt_id": 301
  },
  "summary": "Deployment of api-prod to v1.4.2 succeeded"
}
```

---

### `watcher.deployment_failed`

* **Trigger**: Fired when a deployment attempt hits an unrecoverable failure or times out during installation or health checks.
* **Behavior**:
  - Fired *immediately* when the failure occurs, before any automatic recovery rollback is initiated.
  - The `failure_phase` field informs you of where the sequence broke down (e.g., `download` vs `health_check`).
* **Field contract**:

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `watcher.id` | `integer` | Internal ID of the watcher that emitted the event. |
| `watcher.name` | `string` | Human-facing watcher name. |
| `attempt.id` | `integer` | Unique ID of this failed deploy attempt record. |
| `attempt.kind` | `string` | Always `deploy` for this event. |
| `attempt.reason` | `string` | Why the deploy was started, such as `new_version_found` or `manual_redeploy`. |
| `attempt.triggered_by` | `string` | Who initiated the deploy flow, such as `agent` or `manual`. |
| `attempt.status` | `string` | Always `failed` for this event. |
| `attempt.target_version` | `string` | Version Watcher tried to deploy. |
| `attempt.from_version` | `string` | Version that was active before the failed deploy started. |
| `attempt.failure_phase` | `string` | Step where the deploy failed, such as `download`, `extract`, `activate_release`, `start_services`, or `health_check`. |
| `attempt.error` | `string` | Operator-facing failure detail. Do not automate from this text. |
| `attempt.parent_attempt_id` | `integer \| null` | Immediate parent attempt if this deploy belongs to another attempt chain. Usually `null` for the initial failed deploy. |
| `attempt.root_attempt_id` | `integer` | Root incident chain ID used to correlate related attempts. |
| `summary` | `string` | Human-readable summary for logs and UI. Do not automate from this text. |
* **Interpretation**:
  - This event is emitted before any automatic rollback attempt runs.
  - `failure_phase` is the structured field to automate against, not the free-form `error` text.
  - A later rollback event may follow as part of the same incident chain.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0deployfail",
  "event_type": "watcher.deployment_failed",
  "occurred_at": "2026-06-17T02:21:17Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 302,
    "kind": "deploy",
    "reason": "new_version_found",
    "status": "failed",
    "triggered_by": "agent",
    "target_version": "v1.4.3",
    "from_version": "v1.4.2",
    "failed_target_version": "",
    "failure_phase": "health_check",
    "error": "health check returned 503",
    "parent_attempt_id": null,
    "root_attempt_id": 302
  },
  "summary": "Deployment of api-prod to v1.4.3 failed during health_check"
}
```

---

### `watcher.rollback_succeeded`

* **Trigger**: Fired when a recovery rollback attempt successfully finishes.
* **Behavior**:
  - Triggered for both automated agent rollbacks (after a failed deploy) and manual operator rollbacks.
  - The `failed_target_version` identifies which release version caused the problem, while the `target_version` indicates the safe backup version that was successfully restored.
* **Field contract**:

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `watcher.id` | `integer` | Internal ID of the watcher that emitted the event. |
| `watcher.name` | `string` | Human-facing watcher name. |
| `attempt.id` | `integer` | Unique ID of this rollback attempt record. |
| `attempt.kind` | `string` | Always `rollback` for this event. |
| `attempt.reason` | `string` | Why the rollback was started, such as `auto_after_failed_deploy` or `manual_rollback`. |
| `attempt.triggered_by` | `string` | Who initiated the rollback flow, such as `agent` or `manual`. |
| `attempt.status` | `string` | Always `succeeded` for this event. |
| `attempt.target_version` | `string` | Version that Watcher restored successfully. This is the rollback destination. |
| `attempt.failed_target_version` | `string` | Version whose failure triggered the rollback attempt. |
| `attempt.parent_attempt_id` | `integer \| null` | Immediate parent attempt that caused this rollback. Usually the failed deploy attempt for automatic rollback. |
| `attempt.root_attempt_id` | `integer` | Root incident chain ID used to correlate deploy failure and rollback success together. |
| `summary` | `string` | Human-readable summary for logs and UI. Do not automate from this text. |
* **Interpretation**:
  - `target_version` is the version that was restored.
  - `failed_target_version` is the bad version that triggered recovery.
  - Use `parent_attempt_id` or `root_attempt_id` to correlate this rollback with the original failed deploy.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0rollbackok",
  "event_type": "watcher.rollback_succeeded",
  "occurred_at": "2026-06-17T02:21:52Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 303,
    "kind": "rollback",
    "reason": "auto_after_failed_deploy",
    "status": "succeeded",
    "triggered_by": "agent",
    "target_version": "v1.4.2",
    "from_version": "v1.4.3",
    "failed_target_version": "v1.4.3",
    "failure_phase": "",
    "error": "",
    "parent_attempt_id": 302,
    "root_attempt_id": 302
  },
  "summary": "Rollback of api-prod restored v1.4.2 after v1.4.3 failed"
}
```

---

### `watcher.rollback_failed`

* **Trigger**: Fired when a rollback attempt itself fails (e.g. could not restore the previous junction link or start backup services).
* **Behavior**:
  - This is distinct from `watcher.deployment_failed`. Receivers should expect the deploy failure event first, then this if recovery also breaks.
  - Use the parent/root attempt linkage to correlate the rollback failure back to the deploy incident.
* **Field contract**:

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `watcher.id` | `integer` | Internal ID of the watcher that emitted the event. |
| `watcher.name` | `string` | Human-facing watcher name. |
| `attempt.id` | `integer` | Unique ID of this rollback attempt record. |
| `attempt.kind` | `string` | Always `rollback` for this event. |
| `attempt.reason` | `string` | Why the rollback was started, such as `auto_after_failed_deploy` or `manual_rollback`. |
| `attempt.triggered_by` | `string` | Who initiated the rollback flow, such as `agent` or `manual`. |
| `attempt.status` | `string` | Always `failed` for this event. |
| `attempt.target_version` | `string` | Version Watcher attempted to restore. This is the intended rollback destination. |
| `attempt.failed_target_version` | `string` | Version whose failure led to this rollback attempt. In automatic recovery this is usually the bad deployment version. |
| `attempt.error` | `string` | Operator-facing rollback failure detail. Do not automate from this text. |
| `attempt.parent_attempt_id` | `integer \| null` | Immediate parent attempt that caused this rollback. Usually the failed deploy attempt for automatic rollback. |
| `attempt.root_attempt_id` | `integer` | Root incident chain ID used to correlate deploy failure and rollback failure together. |
| `summary` | `string` | Human-readable summary for logs and UI. Do not automate from this text. |
* **Interpretation**:
  - `target_version` is what Watcher tried to restore.
  - `failed_target_version` is the bad version that triggered recovery.
  - This is usually the highest-severity event in a deployment incident because recovery also failed.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0rollbackfail",
  "event_type": "watcher.rollback_failed",
  "occurred_at": "2026-06-17T02:22:35Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 304,
    "kind": "rollback",
    "reason": "auto_after_failed_deploy",
    "status": "failed",
    "triggered_by": "agent",
    "target_version": "v1.4.2",
    "from_version": "v1.4.3",
    "failed_target_version": "v1.4.3",
    "failure_phase": "activate_release",
    "error": "failed to swap current junction",
    "parent_attempt_id": 302,
    "root_attempt_id": 302
  },
  "summary": "Rollback of api-prod to v1.4.2 failed"
}
```

---

### `service.health_changed`

* **Trigger**: Fired when the recorded health status of a service switches (e.g. `healthy` to `unhealthy`, or vice versa).
* **Behavior**:
  - Fired only on state changes. Repetitive checks showing the same status (e.g., healthy to healthy) are suppressed.
  - The `health.source` field indicates what check triggered this state transition (currently `manual` via health API).
* **Field contract**:

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `watcher.id` | `integer` | Internal ID of the watcher that owns the service. |
| `watcher.name` | `string` | Human-facing watcher name. |
| `service.id` | `integer` | Internal ID of the service whose health changed. |
| `service.name` | `string` | Human-facing service name. |
| `service.service_type` | `string` | Service mode such as `nssm` or `static`. |
| `service.health_check_url` | `string` | Effective health-check URL used for this service. |
| `health.previous_status` | `string` | Previously stored health status before this transition. |
| `health.current_status` | `string` | Newly stored health status after this transition. |
| `health.http_status` | `integer` | HTTP status returned by the health-check endpoint when available. |
| `health.error` | `string` | Operator-facing health-check error detail. Usually empty when the service is healthy. |
| `health.checked_at` | `string` (`RFC3339 date-time`) | When the health check result was recorded. |
| `health.source` | `string` | What flow produced the health change, such as `manual` today and potentially `deploy` or `monitor` in the future. |
| `summary` | `string` | Human-readable summary for logs and UI. Do not automate from this text. |
* **Interpretation**:
  - This event is emitted only when the stored health state changes.
  - The primary subject is the service; watcher context is included for correlation.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0healthchanged",
  "event_type": "service.health_changed",
  "occurred_at": "2026-06-17T03:02:11Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "service": {
    "id": 87,
    "name": "api-prod-web",
    "service_type": "nssm",
    "health_check_url": "https://api.example.com/health"
  },
  "health": {
    "previous_status": "healthy",
    "current_status": "unhealthy",
    "http_status": 503,
    "error": "unexpected status code 503",
    "checked_at": "2026-06-17T03:02:11Z",
    "source": "manual"
  },
  "summary": "Service api-prod-web health changed from healthy to unhealthy"
}
```

---

### `watcher.webhook_test`

* **Trigger**: Fired when an operator clicks the "Send Test Webhook" button in the Watcher UI dashboard.
* **Behavior**:
  - Always fires and bypasses the business-event subscription checkboxes (is not gated).
  - Follows the identical outbox delivery, retrying, and failure logs pipeline as normal webhooks. Use this to verify network connection and header values.
* **Field contract**:

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `watcher.id` | `integer` | Internal ID of the watcher that emitted the test event. |
| `watcher.name` | `string` | Human-facing watcher name. |
| `triggered_by` | `string` | Who triggered the test event. Currently `manual`. |
| `summary` | `string` | Human-readable summary for logs and UI. Do not automate from this text. |
* **Interpretation**:
  - This is a synthetic validation event, not a business lifecycle event.
  - It uses the same outbox and delivery pipeline as real events.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0testhook",
  "event_type": "watcher.webhook_test",
  "occurred_at": "2026-06-17T03:30:00Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "triggered_by": "manual",
  "summary": "Webhook test for watcher api-prod"
}
```

---

### `webhook.delivery_exhausted`

* **Trigger**: Fired when an outbound webhook event completely exhausts all its retry attempts and still fails to deliver.
* **Behavior**:
  - Emitted specifically for failed deliveries, not for suppressed events while paused.
  - Non-recursive: if this exhaustion event fails to deliver, Watcher logs the failure locally but does not emit another notification to prevent a cascading loops.
* **Field contract**:

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `watcher.id` | `integer` | Internal ID of the watcher whose webhook delivery exhausted retries. |
| `watcher.name` | `string` | Human-facing watcher name. |
| `failed_delivery.event_id` | `string` | Business-event ID of the original event that could not be delivered. |
| `failed_delivery.event_type` | `string` | Event type of the original undelivered event, such as `watcher.deployment_failed`. |
| `failed_delivery.delivery_id` | `string` | Delivery-attempt ID of the final failed HTTP attempt. |
| `failed_delivery.attempt_number` | `integer` | Ordinal number of the final failed delivery attempt. |
| `failed_delivery.response_status_code` | `integer` | HTTP status code from the final failed attempt when one was received. |
| `failed_delivery.error` | `string` | Operator-facing delivery failure detail for the final attempt. |
| `failed_delivery.summary` | `string` | Human-readable summary of the original event that could not be delivered. |
| `summary` | `string` | Human-readable summary for logs and UI. Do not automate from this text. |
* **Interpretation**:
  - This event describes a webhook delivery failure, not a service or deployment lifecycle change.
  - Use `failed_delivery.event_id` to correlate back to the original business event.
* **Payload Structure**:
```json
{
  "schema_version": "v1",
  "event_id": "evt_01j0exhausted",
  "event_type": "webhook.delivery_exhausted",
  "occurred_at": "2026-06-17T03:35:27Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "failed_delivery": {
    "event_id": "evt_01j0deployfail",
    "event_type": "watcher.deployment_failed",
    "delivery_id": "dlv_01j0deployfail_4",
    "attempt_number": 4,
    "response_status_code": 503,
    "error": "server returned 503",
    "summary": "Deployment of api-prod to v1.4.3 failed during health_check"
  },
  "summary": "Webhook delivery exhausted for watcher.deployment_failed"
}
```

---

## 6. Event Status vs. Delivery Status Reference

To query webhooks via the API, it is helpful to distinguish between the **Event Outbox Status** and the **Delivery Attempt Status**:

### Event Outbox Status (`webhook_events` Table)

- `pending`: The event is queued and waiting for dispatch or currently being tried.
- `delivered`: The event was successfully received by the target URL (returned a `2xx` response).
- `suppressed`: Outbound delivery was paused when this event occurred. The event is stored and won't be sent until the webhook is resumed and the operator selects to replay suppressed events.
- `exhausted`: The event reached the maximum number of retries or received a final non-retryable status (such as `400` or `403`), and has failed permanently.

### Delivery Attempt Status (`webhook_deliveries` Table)

- `pending`: The network dispatch routine has been spawned but the request is not completed yet.
- `retry_wait`: The dispatch failed with a retryable error. The delivery is waiting in the retry queue according to the retry schedule.
- `succeeded`: The request succeeded with a `2xx` response code.
- `failed`: The attempt failed due to network error, timeout, or a non-2xx status code.

---

## 7. Webhook API Reference

Operators can query webhook states and logs using the following endpoints:

* **List Webhook Events for Watcher**
  - Endpoint: `GET /api/watchers/:id/webhook-events`
  - Description: Returns a list of all outbox events queued or delivered for a watcher.

* **List Delivery Attempts for Watcher**
  - Endpoint: `GET /api/watchers/:id/webhook-deliveries?page=1&pageSize=20`
  - Description: Returns a paginated list of all HTTP request attempts.

* **Retrieve Delivery Attempt Details**
  - Endpoint: `GET /api/watchers/:id/webhook-deliveries/:deliveryId`
  - Description: Returns detailed log entries, response status, headers, and duration for a single delivery attempt.

* **Trigger Test Webhook**
  - Endpoint: `POST /api/watchers/:id/webhook/test`
  - Description: Immediately queues a `watcher.webhook_test` event.

* **Resume Webhook Delivery**
  - Endpoint: `POST /api/watchers/:id/webhook/resume`
  - Input: `{"replay_suppressed": true}`
  - Description: Resumes a paused outbox and optionally triggers sequential replay of all `suppressed` events.
