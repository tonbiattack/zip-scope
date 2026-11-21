// Virtual grid geometry. Kept pure so it can be unit tested without a DOM.

export interface GridGeometry {
  columns: number
  cellWidth: number
  cellHeight: number
  rowCount: number
  totalHeight: number
}

export interface GridInput {
  width: number
  itemCount: number
  thumbSize: number
  gap: number
  labelHeight: number
}

// computeGrid lays out square thumbnails in as many columns as fit.
export function computeGrid(input: GridInput): GridGeometry {
  const { width, itemCount, thumbSize, gap, labelHeight } = input
  const cellWidth = thumbSize + gap
  const cellHeight = thumbSize + labelHeight + gap
  const usable = Math.max(cellWidth, width)
  const columns = Math.max(1, Math.floor(usable / cellWidth))
  const rowCount = Math.ceil(itemCount / columns)
  return {
    columns,
    cellWidth,
    cellHeight,
    rowCount,
    totalHeight: rowCount * cellHeight,
  }
}

export interface VisibleRange {
  start: number
  end: number
  firstRow: number
  lastRow: number
}

// visibleRange returns the half-open index range that intersects the viewport,
// with an overscan of `overscan` rows on each side.
export function visibleRange(
  scrollTop: number,
  viewportHeight: number,
  geometry: GridGeometry,
  itemCount: number,
  overscan = 2,
): VisibleRange {
  if (itemCount === 0 || geometry.columns < 1) {
    return { start: 0, end: 0, firstRow: 0, lastRow: 0 }
  }
  const firstRow = Math.max(0, Math.floor(scrollTop / geometry.cellHeight) - overscan)
  const lastRow = Math.min(
    geometry.rowCount - 1,
    Math.ceil((scrollTop + viewportHeight) / geometry.cellHeight) + overscan,
  )
  const start = Math.min(itemCount, firstRow * geometry.columns)
  const end = Math.min(itemCount, (lastRow + 1) * geometry.columns)
  return { start, end: Math.max(start, end), firstRow, lastRow }
}

// indexAtPoint maps a click position to an item index, or -1 outside the grid.
export function indexAtPoint(
  x: number,
  y: number,
  scrollTop: number,
  geometry: GridGeometry,
  itemCount: number,
): number {
  const localY = y + scrollTop
  const row = Math.floor(localY / geometry.cellHeight)
  const col = Math.floor(x / geometry.cellWidth)
  if (row < 0 || col < 0 || col >= geometry.columns) return -1
  const idx = row * geometry.columns + col
  return idx >= 0 && idx < itemCount ? idx : -1
}
