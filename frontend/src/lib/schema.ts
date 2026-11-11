import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { api } from './api'
import { useSettings } from './store'
import type { Column, Filter, Schema, Values } from './types'

export function useSchema() {
  return useQuery({ queryKey: ['schema'], queryFn: api.schema, staleTime: Infinity, retry: 1 })
}

export function useHealth() {
  return useQuery({ queryKey: ['health'], queryFn: api.health, refetchInterval: 30_000, retry: 0 })
}

/** Looks up columns by name. */
export function useColumns(schema: Schema | undefined) {
  return useMemo(() => {
    const map = new Map<string, Column>()
    for (const c of schema?.columns ?? []) map.set(c.name, c)
    return map
  }, [schema])
}

/** Columns a person can enter about themselves (outcomes excluded). */
export function isPersonalColumn(c: Column): boolean {
  return c.kind !== 'text' && c.role !== 'outcome'
}

/** Columns that can split or group data. */
export function isGroupable(c: Column): boolean {
  if (c.kind === 'text') return false
  if (c.kind === 'continuous') return true
  return (c.levels?.length ?? 0) <= 12
}

export const isNumeric = (c: Column) => c.kind === 'binary' || c.kind === 'discrete' || c.kind === 'continuous'

/** The person's values that are valid for the current schema. */
export function useMyValues(): { values: Values; count: number } {
  const values = useSettings((s) => s.values)
  return { values, count: Object.keys(values).length }
}

/** Filters describing "people like me": same gender, age within five years. */
export function likeMeFilters(values: Values): Filter[] | null {
  const filters: Filter[] = []
  if (typeof values.gender === 'string') filters.push({ column: 'gender', equals: [values.gender] })
  if (typeof values.age === 'number') filters.push({ column: 'age', min: values.age - 5, max: values.age + 5 })
  return filters.length > 0 ? filters : null
}

export { keepPreviousData }
