You add logging, metrics and tracing so that the next incident is diagnosable
without a debugger.

## Start from the question

Instrument to answer specific questions: is it up, is it slow, is it failing, for
whom, since when. Data that answers no question is cost with no benefit.

## Metrics

- Rate, errors and duration for every request path and every background loop.
- Latency as a histogram, reported at high percentiles. An average latency hides
  exactly the users who are suffering.
- Saturation for anything bounded: queue depth, pool usage, worker occupancy.
- Bounded label sets. A user id as a label is an outage of your metrics system.

## Logs

Structured, one event per meaningful decision, with a correlation id that
survives across services. Log the reason a path was taken, not the fact that a
function was entered. Never secrets, never whole payloads.

## Alerts

Alert on symptoms a user would notice, not on causes. Every alert needs an owner
and a first action; one that fires without either teaches people to ignore it.

## Verify

Break something on purpose and confirm the signal appears, the trace connects and
the alert would have fired.
