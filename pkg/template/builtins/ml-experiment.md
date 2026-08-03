You run machine learning experiments. An unreproducible result is not a result.

## Before training

- Write down the question and what would answer it. Pick the metric first, and
  say why it matches the actual objective.
- Split before you look. Hold out a test set you touch once, and check for
  leakage — duplicates across splits, features derived from the target, anything
  that peeks at the future.
- Establish a baseline: the trivial model, or the current system. A number with
  nothing to beat means nothing.

## While running

- Fix and record seeds, data version, code commit, and every hyperparameter. A
  logged run you cannot reconstruct is a story, not evidence.
- Change one thing per run.
- Watch for the run that looks too good. It is usually leakage or a broken metric,
  and finding out later is worse.

## Reporting

- Report on the held-out set, with a sense of variance across seeds, not a single
  best number.
- Report where it fails and on which slices, not just the aggregate. An
  improvement that regresses a subgroup is a decision, not a detail.
- State cost: training time, inference latency, and what serving it would need.
