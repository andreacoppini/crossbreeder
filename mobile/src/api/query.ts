/**
 * Builders for SmartZone's `POST /query/*` endpoints.
 *
 * These are the endpoints that matter on a phone: they are the only ones that
 * search, sort and page server-side, so a 3,000-AP cluster costs one request
 * for the screenful actually on show instead of a full download. The `GET`
 * collection endpoints are for configuration objects, which are few.
 */

import type { SmartZoneClient } from './client';

/**
 * `filters` and `extraFilters` accept *different* sets of types, and the
 * controller enforces the difference: `CLIENT` or `SSID` in `filters` is a
 * 400, not a filter that is quietly ignored.
 *
 * Both enums were read off a SmartZone 7.1.1 cluster by sending a
 * deliberately invalid type and reading the accepted set back out of the
 * error message.
 */

/** Valid in `filters`: the scope dimensions. */
export type QueryScopeFilterType =
  | 'CONTROLBLADE'
  | 'DOMAIN'
  | 'ZONE'
  | 'APGROUP'
  | 'AP'
  | 'INDOORMAP'
  | 'SYNCEDSTATUS'
  | 'REGISTRATIONSTATE';

/** Valid in `extraFilters`: the attribute dimensions, a superset. */
export type QueryExtraFilterType =
  | QueryScopeFilterType
  | 'DATABLADE'
  | 'DATABLADEIPADDRESS'
  | 'THIRD_PARTY_ZONE'
  | 'WLANGROUP'
  | 'WLAN'
  | 'WLANID'
  | 'SSID'
  | 'CLIENT'
  | 'CLIENTIPADDRESS'
  | 'APIPADDRESS'
  | 'OSTYPE'
  | 'STATUS'
  | 'CATEGORY'
  | 'RADIOID'
  | 'PORT'
  | 'APP'
  | 'GATEWAY'
  | 'TIMERANGE'
  | 'CP'
  | 'DP'
  | 'CLUSTER'
  | 'NODE'
  | 'BLADE';

export type QueryFilterType = QueryExtraFilterType;

export interface QueryFilter {
  type: QueryFilterType;
  value: string;
  operator?: 'eq' | 'ne' | 'gt' | 'lt' | 'ge' | 'le' | 'like';
}

/** A filter that is safe to put in the `filters` slot. */
export interface QueryScopeFilter extends QueryFilter {
  type: QueryScopeFilterType;
}

export interface SortInfo {
  sortColumn: string;
  dir: 'ASC' | 'DESC';
}

export interface QueryCriteria {
  /** 1-based. SmartZone pages these endpoints from one, not zero. */
  page?: number;
  limit?: number;
  fullTextSearch?: { type: 'AND' | 'OR'; value: string };
  /**
   * Deliberately unused. A 7.1.1 cluster answers an `attributes` projection
   * with rows missing the very fields it was asked for, so asking for less
   * costs correctness and saves nothing worth having.
   */
  attributes?: string[];
  sortInfo?: SortInfo;
  filters?: QueryFilter[];
  extraFilters?: QueryFilter[];
  extraNotFilters?: QueryFilter[];
  options?: Record<string, unknown>;
}

export interface QueryResult<T> {
  totalCount: number;
  hasMore: boolean;
  firstIndex: number;
  list: T[];
}

export const DEFAULT_PAGE_SIZE = 50;

export interface BuildCriteriaInput {
  page?: number;
  pageSize?: number;
  /** Free-text box contents. Blank and whitespace-only are dropped. */
  search?: string;
  sort?: SortInfo;
  filters?: (QueryFilter | undefined | null | false)[];
  extraFilters?: (QueryFilter | undefined | null | false)[];
  attributes?: string[];
  options?: Record<string, unknown>;
}

/**
 * Turn the state of a list screen into a query body.
 *
 * Empty arrays are omitted rather than sent: some SmartZone builds treat an
 * empty `filters: []` as "match nothing" rather than "no filter".
 */
export function buildCriteria(input: BuildCriteriaInput = {}): QueryCriteria {
  const filters = compact(input.filters);
  const extraFilters = compact(input.extraFilters);
  const search = input.search?.trim();

  const criteria: QueryCriteria = {
    page: input.page ?? 1,
    limit: input.pageSize ?? DEFAULT_PAGE_SIZE,
  };

  if (search) criteria.fullTextSearch = { type: 'AND', value: search };
  if (input.sort) criteria.sortInfo = input.sort;
  if (filters.length > 0) criteria.filters = filters;
  if (extraFilters.length > 0) criteria.extraFilters = extraFilters;
  if (input.attributes && input.attributes.length > 0) {
    criteria.attributes = input.attributes;
  }
  if (input.options && Object.keys(input.options).length > 0) {
    criteria.options = input.options;
  }
  return criteria;
}

function compact(
  values: (QueryFilter | undefined | null | false)[] | undefined,
): QueryFilter[] {
  if (!values) return [];
  return values.filter((v): v is QueryFilter => Boolean(v));
}

/** Run one page of a `/query/*` endpoint. */
export function runQuery<T>(
  client: SmartZoneClient,
  path: string,
  criteria: QueryCriteria,
  signal?: AbortSignal,
): Promise<QueryResult<T>> {
  return client.post<QueryResult<T>>(path, criteria, { signal });
}

/**
 * Walk every page of a `/query/*` endpoint.
 *
 * Used for exports and for the handful of places that genuinely need the
 * whole set (a DPSK CSV, say). `maxPages` keeps a mistake from turning into
 * a hundred requests over a hotel connection.
 */
export async function runQueryAll<T>(
  client: SmartZoneClient,
  path: string,
  criteria: QueryCriteria,
  opts: { maxPages?: number; signal?: AbortSignal } = {},
): Promise<T[]> {
  const maxPages = opts.maxPages ?? 20;
  const limit = criteria.limit ?? 250;
  const out: T[] = [];
  for (let page = 1; page <= maxPages; page += 1) {
    const res = await runQuery<T>(
      client,
      path,
      { ...criteria, page, limit },
      opts.signal,
    );
    out.push(...(res.list ?? []));
    if (!res.hasMore || (res.list?.length ?? 0) === 0) break;
  }
  return out;
}

/** Convenience: page N of a query, given the same input a screen holds. */
export function queryPage<T>(
  client: SmartZoneClient,
  path: string,
  input: BuildCriteriaInput,
  signal?: AbortSignal,
): Promise<QueryResult<T>> {
  return runQuery<T>(client, path, buildCriteria(input), signal);
}
