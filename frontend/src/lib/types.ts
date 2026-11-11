// Types mirroring the Go API's JSON (backend/internal/query, dataset, assistant).

export type Kind = 'binary' | 'discrete' | 'continuous' | 'categorical' | 'text'
export type Tone = 'ok' | 'watch' | 'alert' | 'info'
export type Strength = 'none' | 'weak' | 'moderate' | 'strong'

export interface Condition {
  column: string
  equals: string
}

export interface Band {
  label: string
  min?: number
  max?: number
  tone: Tone
  when?: Condition
}

export interface Range {
  label: string
  min?: number
  max?: number
}

export interface Stats {
  count: number
  missing: number
  min: number
  max: number
  mean: number
  std: number
  p5: number
  p25: number
  median: number
  p75: number
  p95: number
}

export interface Column {
  name: string
  label: string
  short: string
  kind: Kind
  section: string
  role: string
  unit?: string
  description?: string
  decimals: number
  levels?: string[]
  counts?: number[]
  reference?: string
  ordered?: boolean
  bands?: Band[]
  groups?: Range[]
  stats?: Stats
  input?: { min: number; max: number; step: number }
  missing: number
}

export interface Section {
  id: string
  label: string
  description?: string
  columns: string[]
}

export interface OutcomeDef {
  column: string
  equals?: string[]
  min?: number
  max?: number
}

export interface OutcomePreset {
  id: string
  label: string
  description?: string
  outcome: OutcomeDef
}

export interface Schema {
  dataset: {
    name: string
    description?: string
    rows: number
    source: { file: string; sha256: string; bytes: number; generatedAt: string }
  }
  sections: Section[]
  columns: Column[]
  outcomes: OutcomePreset[]
  minGroupSize: number
}

export interface AssistantStatus {
  ready: boolean
  provider: string
  model?: string
  reason?: string
}

export interface Health {
  status: string
  rows: number
  columns: number
  assistant: AssistantStatus
}

export interface Filter {
  column: string
  equals?: string[]
  min?: number
  max?: number
}

export interface Summary {
  n: number
  mean: number
  std: number
  min: number
  p5: number
  p25: number
  median: number
  p75: number
  p95: number
  max: number
  meanLow: number
  meanHigh: number
}

export interface Rate {
  events: number
  n: number
  value: number
  low: number
  high: number
}

export interface Ratio {
  value: number
  low: number
  high: number
}

// Distribution
export interface DistributionRequest {
  column: string
  filters?: Filter[]
  splitBy?: string
  bins?: number
}
export interface Bin {
  start: number
  end: number
}
export interface DistGroup {
  key: string
  label: string
  n: number
  counts: number[]
  shares: number[]
  summary?: Summary
  lowN?: boolean
}
export interface BandShare {
  label: string
  tone: Tone
  min?: number
  max?: number
  count: number
  share: number
}
export interface DistributionResponse {
  column: string
  label: string
  kind: Kind
  mode: 'histogram' | 'levels'
  bins?: Bin[]
  levels?: string[]
  cohort: string
  n: number
  overall: DistGroup
  splitBy?: string
  splitByLabel?: string
  groups?: DistGroup[]
  bands?: BandShare[]
}

// Compare
export interface CompareRequest {
  metric: string
  groupBy: string
  filters?: Filter[]
}
export interface CompareGroup {
  key: string
  label: string
  n: number
  summary?: Summary
  rate?: Rate
  shares?: number[]
  lowN?: boolean
}
export interface TestResult {
  name: string
  statistic: number
  df: number[]
  pValue: number
  effect: number
  effectName: string
  strength: Strength
}
export interface CompareResponse {
  metric: string
  metricLabel: string
  metricKind: Kind
  mode: 'numeric' | 'rate' | 'shares'
  levels?: string[]
  groupBy: string
  groupByLabel: string
  cohort: string
  n: number
  overall: CompareGroup
  groups: CompareGroup[]
  test: TestResult
  summary: string
}

// Risk
export interface Outcome {
  preset?: string
  column?: string
  equals?: string[]
  min?: number
  max?: number
}
export interface OutcomeInfo {
  id?: string
  label: string
  column: string
  definition: string
}
export interface RiskLevel {
  key: string
  label: string
  rate: Rate
  relativeRisk?: Ratio
  difference: number
  isReference?: boolean
  lowN?: boolean
}
export interface Association {
  chiSquare: number
  df: number
  pValue: number
  cramersV: number
  spread: number
  strength: Strength
}
export interface Stratum {
  key: string
  label: string
  n: number
  base: Rate
  levels: RiskLevel[]
  association: Association
}
export interface RiskRequest {
  outcome: Outcome
  factor: string
  reference?: string
  filters?: Filter[]
  stratifyBy?: string
}
export interface RiskResponse {
  outcome: OutcomeInfo
  factor: string
  factorLabel: string
  grouping: string
  reference: string
  cohort: string
  n: number
  base: Rate
  levels: RiskLevel[]
  association: Association
  verdict: string
  stratifyBy?: string
  stratifyByLabel?: string
  strata?: Stratum[]
}
export interface RankRequest {
  outcome: Outcome
  filters?: Filter[]
  includeMarkers?: boolean
}
export interface RankLevel {
  label: string
  rate: number
  n: number
  lowN?: boolean
}
export interface RankedFactor {
  column: string
  label: string
  section: string
  role: string
  association: Association
  levels: RankLevel[]
  highest: RankLevel
  lowest: RankLevel
  verdict: string
}
export interface RankResponse {
  outcome: OutcomeInfo
  cohort: string
  n: number
  base: Rate
  factors: RankedFactor[]
  skipped?: string[]
}

// Profile & similar
export type Values = Record<string, number | string>

export interface ProfileRequest {
  values: Values
  filters?: Filter[]
  columns?: string[]
}
export interface ProfileItem {
  column: string
  label: string
  section: string
  kind: Kind
  unit?: string
  value: number | string
  display: string
  percentile?: number
  median?: number
  p25?: number
  p75?: number
  share?: number
  position?: 'lower' | 'typical' | 'higher'
  band?: Band
  tone?: Tone
}
export interface ProfileResponse {
  cohort: string
  n: number
  items: ProfileItem[]
  invalid?: Record<string, string>
}
export type MatchOn = 'all' | 'lifestyle'
export interface SimilarRequest {
  values: Values
  k?: number
  matchOn?: MatchOn
  outcomes?: Outcome[]
}
export interface SimilarOutcome {
  outcome: OutcomeInfo
  similar: Rate
  overall: Rate
  ratio: number
}
export interface SimilarValue {
  column: string
  label: string
  kind: Kind
  matched: boolean
  your?: number | string
  yourDisplay?: string
  median?: number
  p25?: number
  p75?: number
  top?: string
  topShare?: number
}
export interface SimilarResponse {
  k: number
  matchOn: MatchOn
  features: string[]
  outcomes: SimilarOutcome[]
  values: SimilarValue[]
  invalid?: Record<string, string>
}

// Correlations & relationships
export interface CorrColumn {
  name: string
  label: string
  short: string
  section: string
  kind: Kind
}
export interface CorrelationResponse {
  columns: CorrColumn[]
  matrix: number[][]
  n: number
  top: { a: string; b: string; r: number }[]
}
export interface RelationshipRequest {
  x: string
  y: string
  filters?: Filter[]
  xBins?: number
  yBins?: number
}
export interface AxisInfo {
  column: string
  label: string
  unit?: string
  kind: Kind
  decimals: number
}
export interface RelationshipResponse {
  x: AxisInfo
  y: AxisInfo
  cohort: string
  n: number
  xBins: Bin[]
  yBins: Bin[]
  cells: { x: number; y: number; count: number }[]
  maxCount: number
  trend: { bin: number; x: number; n: number; mean: number; p25: number; median: number; p75: number }[]
  pearson: number
  slope: number
  intercept: number
  summary: string
}

export interface RecordResponse {
  index: number
  values: Record<string, number | string | null>
}

// Assistant stream
export type ViewKind = 'distribution' | 'compare' | 'risk' | 'rank' | 'profile' | 'similar'
export type View =
  | { kind: 'distribution'; request: DistributionRequest }
  | { kind: 'compare'; request: CompareRequest }
  | { kind: 'risk'; request: RiskRequest }
  | { kind: 'rank'; request: RankRequest }
  | { kind: 'profile'; request: ProfileRequest }
  | { kind: 'similar'; request: SimilarRequest }

export type ChatEvent =
  | { type: 'session'; data: { sessionId: string } }
  | { type: 'delta'; data: { text: string } }
  | { type: 'message'; data: { text: string } }
  | { type: 'tool_call'; data: { id: string; name: string; label: string; args: Record<string, unknown> } }
  | {
      type: 'tool_result'
      data: { id: string; name: string; ok: boolean; error?: string; view?: View; result?: Record<string, unknown> }
    }
  | { type: 'error'; data: { code: string; message: string } }
  | { type: 'done'; data: { elapsedMs?: number; toolCalls?: number } }
