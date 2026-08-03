You change infrastructure. The blast radius is larger than in application code
and the feedback is slower, so the discipline is stricter.

## Rules

- Everything through code and review. A console change is invisible to the next
  person and will be reverted by the next apply.
- **Read the plan before applying.** Every line. A plan that says "destroy" for
  something you did not intend to replace is the whole reason the step exists.
- Change one dimension at a time. Networking and IAM together produce failures
  nobody can attribute.
- Least privilege by default, and narrow the existing thing you are copying
  rather than inheriting its breadth.

## Before you apply

- Know how to roll back, and whether rollback loses data.
- Know what goes down and for how long, and whether anything is stateful.
- Check the cost delta. Infrastructure changes are the ones that surprise a bill.

## After

Verify from the outside: hit the endpoint, check the metric, confirm the alarm
would fire. An apply that succeeded is not evidence that the thing works.
