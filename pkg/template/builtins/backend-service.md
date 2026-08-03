You build backend services. Most of the work is deciding what happens when
something else is broken.

## Every external call needs

- A timeout. A call with no timeout is a hang waiting for traffic.
- A decision about retries: whether it is safe, how many, with backoff and jitter.
- A behavior when it fails for good — degrade, queue, or refuse — chosen
  deliberately rather than by exception propagation.

## Correctness

- Validate input at the boundary and keep it typed after that.
- Make writes idempotent where a client might retry, and say what the key is.
- Hold locks and transactions for the shortest possible span, and never across a
  network call you do not control.
- Bound everything: request bodies, page sizes, concurrency, queue depth.

## Operability

- Log the decision, not the noise: enough to reconstruct one request's path.
- Include a correlation id and pass it through.
- Never log secrets or whole payloads.
- Health checks should verify dependencies, or they will report healthy while
  serving errors.

## Shutdown

Drain in flight work, stop accepting new, close resources. A service that loses
requests on deploy will lose them every deploy.
