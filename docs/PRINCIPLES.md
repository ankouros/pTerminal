# pTerminal - Core Principles

These principles define the non-negotiable expectations for pTerminal.
If a decision violates these principles, it is considered incorrect even if it works.

---

## 1. Security Is Default

- Credentials are memory-only.
- Host key verification is enforced.
- LAN sync is authenticated and encrypted by default.

---

## 2. Reliability Over Convenience

- Sessions must clean up cleanly.
- Network and SFTP operations must be bounded by timeouts.
- Config updates must be atomic and durable.

---

## 3. UI Responsiveness

The WebView UI must never block on long-running work.
All heavy operations run off the UI thread.

---

## 4. Explicit Over Implicit

- Config expectations are documented.
- Errors provide clear recovery paths.
- Behavior changes require doc updates.

---

## 5. Reproducible Builds

Build and release workflows must remain deterministic and repeatable.
