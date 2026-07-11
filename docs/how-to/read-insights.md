# Read Insights

Insights is a single-page analytics dashboard that puts spend, token usage, agent health, and system load in one scrollable view.

## Overview

Open Insights from the web UI at `http://localhost:9374`. Everything lives on one page: a KPI strip at the top, a sticky anchor-nav below it, and a stack of chart panels grouped into sections. The anchor-nav pills smooth-scroll to each section, and the whole page respects the date range you pick.

## The KPI Strip

Five headline numbers sit across the top:

- **Spend (this range)** — total cost over the selected range, summed from the daily cost ledger.
- **Tokens** — total tokens over the range.
- **Active agents** — alive agents over total agents, shown as `X / Y`.
- **Burn rate** — range spend divided by the window, shown as `$X.XX/hr`.
- **Top cost driver** — the agent with the highest all-time spend, with its total beside it.

## The Sections

Below the KPI strip, the anchor-nav jumps between five sections:

| Section | What it shows |
|---------|---------------|
| **Agents** | A table of every agent — name, role, provider, state, CPU, memory, tokens, and cost. |
| **Cost** | Cost over time, cost by agent, and cost by model. |
| **Usage** | Token throughput, model usage by tokens, cache efficiency, and a per-agent token breakdown. |
| **System** | CPU by agent, memory by agent, network I/O, and disk I/O. |
| **Activity** | Notification activity for the top ten busiest channels. |

## Where the Numbers Come From

Every panel reads from the daemon's ledger and stats endpoints — the same `/api/costs/*` and `/api/stats/*` data that back the CLI. Cost and token figures come straight from the cost ledger, so per-agent, per-model, and daily dollars and tokens are real recorded usage, not estimates. System panels (CPU, memory, network, disk) come from the per-agent stats collectors, and the Activity section reads channel delivery counts.

> Tip: use the **Top cost driver** KPI and the **Cost by agent** chart together to spot which agent is spending the most, then open that agent in Insights' Agents table to see its live resource use.
