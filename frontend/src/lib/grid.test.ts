import { describe, expect, it } from 'vitest'
import { computeGrid, indexAtPoint, visibleRange } from './grid'

describe('computeGrid', () => {
  it('fits as many columns as the width allows', () => {
    const g = computeGrid({ width: 800, itemCount: 100, thumbSize: 192, gap: 8, labelHeight: 18 })
    // cellWidth = 200, so 4 columns fit in 800px.
    expect(g.columns).toBe(4)
    expect(g.rowCount).toBe(25)
    expect(g.totalHeight).toBe(25 * (192 + 18 + 8))
  })

  it('always has at least one column', () => {
    const g = computeGrid({ width: 50, itemCount: 10, thumbSize: 192, gap: 8, labelHeight: 18 })
    expect(g.columns).toBe(1)
  })

  it('handles an empty grid', () => {
    const g = computeGrid({ width: 800, itemCount: 0, thumbSize: 192, gap: 8, labelHeight: 18 })
    expect(g.rowCount).toBe(0)
    expect(g.totalHeight).toBe(0)
  })

  it('grows the row count with the item count', () => {
    const a = computeGrid({ width: 800, itemCount: 4, thumbSize: 192, gap: 8, labelHeight: 18 })
    const b = computeGrid({ width: 800, itemCount: 8, thumbSize: 192, gap: 8, labelHeight: 18 })
    expect(b.rowCount).toBe(a.rowCount + 1)
  })
})

describe('visibleRange', () => {
  const geometry = computeGrid({ width: 800, itemCount: 1000, thumbSize: 192, gap: 8, labelHeight: 18 })

  it('returns the first rows at the top', () => {
    const r = visibleRange(0, 600, geometry, 1000, 0)
    expect(r.start).toBe(0)
    expect(r.end).toBeGreaterThan(0)
  })

  it('never renders every item of a large grid', () => {
    const r = visibleRange(0, 600, geometry, 1000, 2)
    expect(r.end - r.start).toBeLessThan(1000)
  })

  it('moves the window when scrolled', () => {
    const top = visibleRange(0, 600, geometry, 1000, 0)
    const mid = visibleRange(2000, 600, geometry, 1000, 0)
    expect(mid.start).toBeGreaterThan(top.start)
  })

  it('clamps at the end of the list', () => {
    const r = visibleRange(1e9, 600, geometry, 1000, 2)
    expect(r.end).toBe(1000)
    expect(r.start).toBeLessThanOrEqual(1000)
  })

  it('returns an empty range for an empty grid', () => {
    const empty = computeGrid({ width: 800, itemCount: 0, thumbSize: 192, gap: 8, labelHeight: 18 })
    const r = visibleRange(0, 600, empty, 0, 2)
    expect(r.start).toBe(0)
    expect(r.end).toBe(0)
  })

  it('includes overscan rows on both sides', () => {
    const noOver = visibleRange(2000, 600, geometry, 1000, 0)
    const withOver = visibleRange(2000, 600, geometry, 1000, 2)
    expect(withOver.start).toBeLessThan(noOver.start)
    expect(withOver.end).toBeGreaterThan(noOver.end)
  })
})

describe('indexAtPoint', () => {
  const geometry = computeGrid({ width: 800, itemCount: 100, thumbSize: 192, gap: 8, labelHeight: 18 })

  it('maps the first cell', () => {
    expect(indexAtPoint(10, 10, 0, geometry, 100)).toBe(0)
  })

  it('maps a cell in the second column', () => {
    expect(indexAtPoint(geometry.cellWidth + 10, 10, 0, geometry, 100)).toBe(1)
  })

  it('accounts for scroll', () => {
    const row = 3
    const y = 10
    const idx = indexAtPoint(10, y, row * geometry.cellHeight, geometry, 100)
    expect(idx).toBe(row * geometry.columns)
  })

  it('returns -1 outside the grid', () => {
    expect(indexAtPoint(-5, 10, 0, geometry, 100)).toBe(-1)
    expect(indexAtPoint(10, -5, 0, geometry, 100)).toBe(-1)
    expect(indexAtPoint(10, 1e6, 0, geometry, 100)).toBe(-1)
  })
})
