import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ArchiveList from './ArchiveList.vue'
import type { ArchiveSummary } from '../lib/types'

function archive(over: Partial<ArchiveSummary> = {}): ArchiveSummary {
  return {
    id: 1,
    path: 'C:\\pics\\book.zip',
    name: 'book.zip',
    size: 1024,
    modTime: new Date(2026, 0, 2, 3, 4).getTime(),
    count: 12,
    status: 'ok',
    coverOrdinal: 0,
    coverWidth: 192,
    ...over,
  }
}

function mountList(archives: ArchiveSummary[], over: Record<string, unknown> = {}) {
  return mount(ArchiveList, {
    props: {
      archives,
      selectedId: null,
      sortMode: 'name' as const,
      sortDesc: false,
      search: '',
      scanning: false,
      ...over,
    },
  })
}

describe('ArchiveList', () => {
  it('renders one row per archive', () => {
    const w = mountList([archive({ id: 1, name: 'a.zip' }), archive({ id: 2, name: 'b.zip' })])
    expect(w.findAll('.row')).toHaveLength(2)
  })

  it('shows the image count and size', () => {
    const w = mountList([archive({ count: 42, size: 2048 })])
    const text = w.text()
    expect(text).toContain('42 枚')
    expect(text).toContain('2.0 KB')
  })

  it('shows a status label instead of a count for a broken archive', () => {
    const w = mountList([archive({ status: 'corrupt', count: 0 })])
    expect(w.text()).toContain('破損')
  })

  it('emits select when a row is clicked', async () => {
    const a = archive()
    const w = mountList([a])
    await w.find('.row').trigger('click')
    expect(w.emitted('select')?.[0]).toEqual([a])
  })

  it('marks the selected row', () => {
    const w = mountList([archive({ id: 7 })], { selectedId: 7 })
    expect(w.find('.row').classes()).toContain('selected')
  })

  it('filters by the search query', () => {
    const w = mountList(
      [archive({ id: 1, name: 'alpha.zip' }), archive({ id: 2, name: 'beta.zip' })],
      { search: 'alpha' },
    )
    expect(w.findAll('.row')).toHaveLength(1)
    expect(w.text()).toContain('alpha.zip')
  })

  it('sorts by name naturally', () => {
    const w = mountList([
      archive({ id: 1, name: 'page10.zip' }),
      archive({ id: 2, name: 'page2.zip' }),
    ])
    const names = w.findAll('.name').map((n) => n.text())
    expect(names).toEqual(['page2.zip', 'page10.zip'])
  })

  it('sorts by size descending when requested', () => {
    const w = mountList(
      [archive({ id: 1, name: 'small.zip', size: 10 }), archive({ id: 2, name: 'big.zip', size: 999 })],
      { sortMode: 'size', sortDesc: true },
    )
    const names = w.findAll('.name').map((n) => n.text())
    expect(names).toEqual(['big.zip', 'small.zip'])
  })

  it('shows the empty message when there is nothing to list', () => {
    const w = mountList([])
    expect(w.text()).toContain('ZIP が見つかりません')
  })

  it('shows a scanning message while the walk runs', () => {
    const w = mountList([], { scanning: true })
    expect(w.text()).toContain('走査中')
  })

  it('emits pick-folder from the toolbar button', async () => {
    const w = mountList([])
    await w.find('.btn.primary').trigger('click')
    expect(w.emitted('pick-folder')).toBeTruthy()
  })

  it('shows a cancel button only while scanning', async () => {
    const idle = mountList([])
    expect(idle.find('.btn.danger').exists()).toBe(false)

    const busy = mountList([], { scanning: true })
    expect(busy.find('.btn.danger').exists()).toBe(true)
    await busy.find('.btn.danger').trigger('click')
    expect(busy.emitted('cancel-scan')).toBeTruthy()
  })

  it('emits the search value as the user types', async () => {
    const w = mountList([])
    await w.find('.search').setValue('hello')
    expect(w.emitted('update:search')?.[0]).toEqual(['hello'])
  })

  it('toggles the sort direction when the active chip is clicked again', async () => {
    const w = mountList([], { sortMode: 'name', sortDesc: false })
    const chips = w.findAll('.chip')
    await chips[0].trigger('click')
    expect(w.emitted('update:sortDesc')?.[0]).toEqual([true])
  })

  it('switches the sort mode when another chip is clicked', async () => {
    const w = mountList([], { sortMode: 'name' })
    const chips = w.findAll('.chip')
    await chips[1].trigger('click')
    expect(w.emitted('update:sortMode')?.[0]).toEqual(['mtime'])
  })

  it('does not render every row for a very large list', () => {
    const many: ArchiveSummary[] = []
    for (let i = 0; i < 5000; i++) {
      many.push(archive({ id: i + 1, name: `book${i}.zip` }))
    }
    const w = mountList(many)
    // Virtual scrolling must keep the DOM small.
    expect(w.findAll('.row').length).toBeLessThan(100)
  })

  it('reports the filtered count', () => {
    const w = mountList([archive({ id: 1 }), archive({ id: 2 }), archive({ id: 3 })])
    expect(w.find('.count').text()).toBe('3 件')
  })
})
