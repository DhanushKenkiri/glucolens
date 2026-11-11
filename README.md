# GlucoLens

**Live site: https://dhanushkenkiri.github.io/glucolens/**

Understand a checkup by comparing it with 100,000 anonymised checkups. Three tabs:

- **Ask** – questions answered by an agent built with Google's [Agent Development Kit for Go](https://github.com/google/adk-go) (the Go edition of [adk-python](https://github.com/google/adk-python)), running on Groq. Every number comes from a live query; each answer shows the charts it was based on, with a link to open them in the other tabs.
- **Insights** – every column charted by section, with your value marked, clinical ranges shaded, cohorts (everyone / people like me / custom filters), split-by, a compare-across-groups view, and a correlation map with a density plot for any pair.
- **Risk Explorer** – pick an outcome (diagnosed diabetes, Type 2, HbA1c ≥ 6.5, hypertension, …), see every factor ranked by strength of link, drill into one factor with 95% confidence intervals, relative risk and stratification (to check confounding), and see outcomes among the 500 people most similar to you.

Your own values are entered under **My values** and saved only in your browser.

> For learning only, not medical advice. About 60% of people in this dataset have diagnosed diabetes, so it is not a sample of the general population; the app always says "in this dataset".

## Architecture

```
React (Vite, :5173) ──/api proxy──> Go + gorilla/mux (:8080)
                                      ├─ dataset    CSV ─once─> dataset.json ─> typed columns in memory
                                      ├─ query      filters, distributions, comparisons, risk, percentiles, similar people
                                      ├─ api        REST handlers (JSON)              ┐ same query
                                      └─ assistant  ADK agent ── six typed tools ─────┘ functions
                                                      └─> Groq (OpenAI-compatible Chat Completions)
```

- **The AI never reads the CSV.** On startup the Go server converts `backend/data/diabetes_dataset.csv` into `backend/data/dataset.json` (only when the JSON is missing or the CSV's SHA-256 changed) and loads the JSON. The agent can only call six tools — `get_distribution`, `compare_groups`, `analyze_risk`, `rank_risk_factors`, `get_percentile`, `find_similar_people` — which run the same query code as the REST API, so charts and answers always agree.
- **Flexible typing.** Column types are inferred from the data: yes/no and 0/1 → binary, small integer scales → discrete, other numbers → continuous, text → categorical (or free text). `backend/internal/dataset/columns.meta.json` adds labels, units, sections, plain-language descriptions, level order and clinical reference bands; columns it doesn't know still work with inferred types.
- **Statistics.** Wilson 95% intervals for rates, log-method relative risk, chi-square with Cramér's V, one-way ANOVA with η, groups under 30 people flagged.

## Run it

Prerequisites: **Go 1.27+** and **Node 22+**.

```bash
npm run setup          # installs JS dependencies and downloads Go modules
```

Add your Groq key (from https://console.groq.com/keys):

```bash
# backend/.env was created from backend/example.env; set:
GROQ_API_KEY=gsk_...
```

The server also reads a `.env` in the project root, and real environment variables override both. Without a key everything except the Ask tab works, and the Ask tab shows these setup steps.

```bash
npm run dev            # Go API on :8080 + Vite on :5173 → open http://localhost:5173
```

Production-style: `npm run build` then `npm start` serves the built frontend and the API together on http://localhost:8080.

### Configuration (`backend/example.env`)

| Variable | Default | Notes |
| --- | --- | --- |
| `GROQ_API_KEY` | – | Required for the Ask tab. |
| `GROQ_MODEL` | `openai/gpt-oss-120b` | Any Groq model with tool use. `openai/gpt-oss-20b` is lighter on rate limits; `llama-3.3-70b-versatile` also works. |
| `GROQ_REASONING_EFFORT` | `low` | Sent only to reasoning models (gpt-oss, qwen3). |
| `GROQ_BASE_URL` | `https://api.groq.com/openai/v1` | |
| `PORT` | `8080` | |
| `ALLOWED_ORIGINS` | `http://localhost:5173,…` | CORS for the dev server. |
| `DATASET_CSV` / `DATASET_JSON` | `data/diabetes_dataset.csv` / `data/dataset.json` | Replace the CSV and restart to reconvert (or run `go run ./cmd/server -reconvert`). |
| `STATIC_DIR` | `../frontend/dist` | Built frontend, served at `/` when present. |

## The static site (GitHub Pages)

GitHub Pages only serves files, so the Pages build runs the Go code **in the browser**: the dataset loader, query engine, gorilla/mux router and ADK agent are compiled to WebAssembly and run in Web Workers. Nothing else changes; the frontend sends the same `/api/v1` requests, and a small runtime (`frontend/src/lib/runtime.ts`) hands them to the worker instead of the network.

- `glucolens-core.wasm` (8 MB) serves the data API, so Insights and Risk Explorer work as soon as the dataset has loaded.
- `glucolens-agent.wasm` (31 MB) adds the ADK agent. It loads when someone uses the Ask tab with a Groq key, then takes over all requests. In the browser the agent talks to Groq through a small built-in client (`internal/assistant/groqlite.go`) instead of ADK's openai-go adapter, which would make the download three times larger.
- **Visitors bring their own Groq key.** The key is entered on the Ask tab, saved only in that browser and sent only to Groq. No key is ever built into the site.

Build and check it locally:

```bash
npm run build:pages          # → frontend/dist-pages
npm run e2e:static           # serves it like GitHub Pages and drives Edge through it
```

**Deployment** is automatic: `.github/workflows/pages.yml` runs on every push to `main` — Go and frontend tests, lint, the static build — and publishes `frontend/dist-pages` to the `gh-pages` branch, which GitHub Pages serves (Settings → Pages → Deploy from a branch → `gh-pages`, folder `/`).

## Checks

```bash
npm test               # Go tests (dataset, query, api, assistant, config) + Vitest
npm run lint           # go vet + ESLint
npm run e2e            # builds, then drives Microsoft Edge through all three tabs
npm run e2e:static     # the same for the GitHub Pages build
```

- The Go tests assert figures computed independently from the CSV (row and level counts, mean age 50.12, diabetes 54.9% vs 78.3% by family history, ~60.0% for every smoking group) and hand-checked statistics.
- The assistant tests run the real ADK runner and OpenAI adapter against a scripted fake Groq server: multi-step tool calls, user values passed through session state, tool errors returned to the model, rate-limit handling, and the exact request bodies Groq would receive.
- The e2e test runs the real Go binary with a fake Groq, at desktop and phone widths in light and dark themes, and fails on any console error or failed request. Screenshots go to `frontend/e2e/output`.

## API

All under `/api/v1`. Errors are `{"error": {"code", "message"}}`; bad column names or values return 400 with the valid options.

| Route | Purpose |
| --- | --- |
| `GET /health` | Rows, columns, assistant status |
| `GET /schema` | Sections, columns (kind, levels, stats, bands), outcome presets |
| `POST /query/distribution` | Histogram or level counts, optional `splitBy`, clinical-band shares |
| `POST /query/compare` | A metric across groups (box-plot stats, rates or shares) with a significance test |
| `POST /query/risk` | Outcome rate per factor group, CIs, relative risk, verdict, optional `stratifyBy` |
| `POST /query/risk/rank` | Every factor ranked by Cramér's V |
| `POST /query/profile` | Percentile, typical range and clinical band for each of your values |
| `POST /query/similar` | Outcomes among the K most similar people |
| `GET /query/correlations` | Correlation matrix of numeric columns |
| `POST /query/relationship` | 2-D density and median trend for two numeric columns |
| `GET\|POST /records`, `GET /records/random` | Rows as JSON; a random row as a sample patient |
| `POST /chat` | The assistant's answer as server-sent events (`session`, `delta`, `message`, `tool_call`, `tool_result`, `error`, `done`) |

Filters are `{"column", "equals": [...]}` or `{"column", "min", "max"}` (inclusive). Outcomes are `{"preset": "diagnosed_diabetes"}` or `{"column", "equals" | "min" | "max"}`.

## Layout

```
backend/
  cmd/server/            entry point (flags: -reconvert, -convert-only, -env)
  cmd/wasm-core/         the data API compiled to WebAssembly (static site)
  cmd/wasm-agent/        data API + ADK agent compiled to WebAssembly (static site)
  internal/chat/         the assistant contract (no ADK dependency)
  internal/wasmbridge/   exposes an http.Handler to JavaScript
  internal/config/       env + .env loading
  internal/dataset/      CSV inference, CSV→JSON conversion, loader, column overlay
  internal/query/        the analysis engine
  internal/api/          gorilla/mux router, handlers, SSE
  internal/assistant/    ADK agent, tools, prompt, event stream
frontend/
  src/components/reactbits/   React Bits components (installed with the shadcn CLI)
  src/components/charts/      SVG charts (histogram, bars, box plots, rate rows, heatmap, density)
  src/components/views/       query-backed views shared by Ask, Insights and Risk Explorer
  src/pages/                  the three tabs
  e2e/                        browser tests (server build and static build) with a fake Groq server
scripts/build-pages.mjs       builds the GitHub Pages site
.github/workflows/pages.yml   tests, builds and deploys on every push to main
```

## Notes

- Conversations are kept in server memory and reset when the server restarts.
- Groq's free tier limits tokens per minute; a question typically takes 2–4 model calls. If you hit the limit, the app says so; switch to a lighter model or wait a minute.
- In this dataset smoking shows no difference in diabetes rates (60.0% for never, former and current smokers); family history, age, activity and BMI do. The app reports that plainly rather than inventing an effect.
- ADK Go's OpenAI adapter is marked experimental; it is isolated in `backend/internal/assistant/agent.go`.
- See [ARCHITECTURE.md](ARCHITECTURE.md) for diagrams of the system, the assistant and error handling.
