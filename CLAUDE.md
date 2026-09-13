# Claude Repository Instructions

## Transient failure recovery

For tool, network, API, connector, filesystem, remote-service, or other potentially transient operational errors:

1. Retry up to **5 total attempts**.
2. Use **exponential backoff** between attempts: approximately **1s, 2s, 4s, 8s**.
3. For any ambiguous write failure (timeout, dropped connection, 5xx after a mutation, unknown result), **verify the target state before retrying**. Never blindly repeat a mutation that may already have succeeded.
4. Stop retrying early when a clear non-transient cause emerges, including invalid input, authentication/authorization failure, semantic validation failure, deterministic conflict, missing required resource, or a condition requiring human judgment.
5. When state verification shows the operation succeeded despite the reported error, continue from the verified state rather than repeating the write.
6. After 5 unsuccessful transient attempts, report the operation, observed failures, and verified final state instead of claiming success.

This recovery pattern applies to repository work and should be preferred over failing immediately on the first transient error.
