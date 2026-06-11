# Reporting module (DOCX mail-merge)

Generic engine that fills a fixed corporate DOCX template with campaign metrics,
text variables and images. It is a **mail-merge over a fixed template**, not a
document builder: it never creates sections, paragraphs, styles or figures, and
never renumbers anything. The template is the absolute source of truth for
layout and formatting.

> **Confidentiality:** this repository contains only the generic engine. No
> corporate template, logo, narrative or sample report is versioned. All such
> content lives in the database (uploaded at runtime) and is excluded from git
> (see `.gitignore`). The engine only becomes a specific corporate report when a
> template is uploaded.

## Pieces

| File | Responsibility |
|------|----------------|
| `source.go` | `ReportSource` interface decoupling the engine from Campaign vs Campaign Group |
| `metrics.go` | `Funnel()` (disjoint results-table buckets), date formatting |
| `twofa.go` | `Validate2FA()` + `Bloque2FA()` (manual aggregate 2FA narrative) |
| `inspect.go` | token/slot discovery + validation on template upload |
| `slots.go` | closed catalog of image slots |
| `variables.go` | `BuildVars()` assembles the token map |
| `chart.go` | `ChartPNG()` renders "Gráfico 1" (the only auto-generated figure) |
| `render.go` | pure `Render()`: template + vars + images → DOCX bytes |
| `blobstore.go` | `BlobStore` abstraction (DB-backed by `models.DBBlobStore`) |

`render.go` is pure: it has no knowledge of HTTP, controllers, users, requests
or responses. It receives a template, variables and images and returns DOCX
bytes.

## Architectural decision: single source of truth for metrics

Every module (campaign dashboard, campaign-group dashboard, CSV exports and
Reporting) must report the **same per-recipient numbers** for a given campaign
or group. Shared rule: each unique recipient is classified **once** by the
furthest funnel stage reached (`models.FunnelRank`), and counts are cumulative
(submitted ⊆ clicked ⊆ opened ⊆ sent).

- `getCampaignStats` (individual campaign) counts **per result** with cumulative
  backfill — unique-recipient. The dashboard JS computes the same from
  `result.status`. Reporting consumes it via `GetCampaignSummary`.
- `GetCampaignGroupStats` **now** computes its Sent/Opened/Clicked/Submitted
  counters **per recipient** (furthest stage across all campaigns of the group),
  not per event. Previously it incremented per event, so a recipient who opened
  several times inflated the counters — that was the source of the
  dashboard-vs-report discrepancy. Calendar metrics stay event-based as a
  separate informational signal.
- Reporting's `campaignGroupSource.Stats()` consumes those counters **directly**,
  guaranteeing the report shows byte-identical numbers to the group dashboard
  and CSV. `funnelInputFromJourneys` remains as the documented/tested reference
  of the same classification (`TestFunnelFromJourneys`).

**Do not** revert `GetCampaignGroupStats` to per-event counting: it would inflate
the funnel and desynchronize the dashboard, CSV, report table, chart, Excel and
conclusions. Covered by `TestCampaignGroupFunnelIsPerRecipientNotPerEvent`.

## 2FA model

ChronoLure/GoPhish cannot determine whether a captured credential was protected
by MFA. 2FA is therefore a single manually-entered aggregate (`UsersWith2FA`),
validated with `Validate2FA` (must not exceed `SubmittedData`). `Bloque2FA`
selects one of three predefined phrasings (all protected / none / mixed). No
per-user findings are stored or inferred.

## Reproducibility

Generated reports are frozen: the exact template version, the metrics snapshot
and the exact image bytes used are persisted so a report can be regenerated
byte-identically regardless of later edits (tables `report_renders` /
`report_render_assets`, backed by content-addressed `report_blobs`).
