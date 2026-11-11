import type {
  CompareRequest,
  CompareResponse,
  CorrelationResponse,
  DistributionRequest,
  DistributionResponse,
  Health,
  ProfileRequest,
  ProfileResponse,
  RankRequest,
  RankResponse,
  RecordResponse,
  RelationshipRequest,
  RelationshipResponse,
  RiskRequest,
  RiskResponse,
  Schema,
  SimilarRequest,
  SimilarResponse,
} from './types'

import { apiFetch, STATIC } from './runtime'

export { API_BASE } from './runtime'

/** What to tell people when the API cannot be reached, for each mode. */
export const UNREACHABLE = STATIC
  ? 'GlucoLens could not load its data in this browser. Reload the page to try again.'
  : 'The GlucoLens API is not reachable. Is the Go server running? (npm run dev starts it.)'

/** An error returned by the API, with its machine-readable code. */
export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export async function errorFromResponse(res: Response): Promise<ApiError> {
  let code = 'http_error'
  let message = `Request failed (${res.status})`
  try {
    const body = await res.json()
    if (body?.error) {
      code = body.error.code ?? code
      message = body.error.message ?? message
    }
  } catch {
    // Not JSON (for example the API is down and the dev proxy answered).
    if (res.status === 502 || res.status === 504 || res.status === 500) {
      code = 'api_unreachable'
      message = UNREACHABLE
    }
  }
  return new ApiError(res.status, code, message)
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await apiFetch(path, init)
  } catch (cause) {
    console.error(`[GlucoLens] API ${path} unreachable`, cause)
    const detail = STATIC && cause instanceof Error && cause.message ? ` (${cause.message})` : ''
    throw new ApiError(0, 'api_unreachable', UNREACHABLE + detail)
  }
  if (!res.ok) {
    const err = await errorFromResponse(res)
    console.error(`[GlucoLens] API ${init?.method ?? 'GET'} ${path} failed`, { status: err.status, code: err.code, message: err.message })
    throw err
  }
  return (await res.json()) as T
}

const get = <T,>(path: string) => request<T>(path)
const post = <T,>(path: string, body: unknown) =>
  request<T>(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })

export const api = {
  health: () => get<Health>('/health'),
  schema: () => get<Schema>('/schema'),
  distribution: (req: DistributionRequest) => post<DistributionResponse>('/query/distribution', req),
  compare: (req: CompareRequest) => post<CompareResponse>('/query/compare', req),
  risk: (req: RiskRequest) => post<RiskResponse>('/query/risk', req),
  rank: (req: RankRequest) => post<RankResponse>('/query/risk/rank', req),
  profile: (req: ProfileRequest) => post<ProfileResponse>('/query/profile', req),
  similar: (req: SimilarRequest) => post<SimilarResponse>('/query/similar', req),
  correlations: () => get<CorrelationResponse>('/query/correlations'),
  relationship: (req: RelationshipRequest) => post<RelationshipResponse>('/query/relationship', req),
  randomRecord: () => get<RecordResponse>('/records/random'),
}
