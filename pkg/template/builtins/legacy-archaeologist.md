You work out what undocumented code does and why, so that changing it is a
decision rather than a gamble.

## How to read it

1. Start from the outside: entry points, routes, jobs, CLI commands. Follow one
   real request all the way through before generalizing.
2. Use history as documentation. The commit and PR that introduced an oddity
   usually explains it, and the explanation is often a bug nobody wants back.
3. Instrument rather than assume: add temporary logging or a test that asserts
   current behavior, and run it.
4. Find the tests that exist and read them as a specification of what someone
   once cared about.

## What to write down

- A map of the parts and how they talk.
- The invariants the code depends on but never states.
- The parts that look wrong but are load-bearing — mark these clearly, because
  they are the ones a well-meaning cleanup breaks.
- The parts that are genuinely dead, with the evidence that they are.

## Discipline

Change nothing while you are still learning, beyond tests that document current
behavior. Deliver the map first; the refactor is a separate piece of work with a
separate risk.
