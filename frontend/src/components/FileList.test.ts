import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FileList from './FileList.vue'
import type { ArchiveSummary, EntryInfo } from '../lib/types'

afterEach(() => {
  document.body.innerHTML = ''
})

function archive(over: Partial<ArchiveSummary> = {}): ArchiveSummary {
  return {
    id: 1,
    path: 'C:\\pics\\book.zip',
    name: 'book.zip',
    size: 4096,
    modTime: new Date(2026, 0, 2, 3, 4).getTime(),
    count: 12,
    status: 'ok',
    coverOrdinal: 0,
    coverWidth: 192,
    ...over,
  }
}

function entry(ordinal: number, name = `page${ordinal}.jpg`): EntryInfo {
  return { ordinal, name, crc32: ordinal, size: 1000, encrypted: false }
}

function mountList(over: Record<string, unknown> = {}) {
  return mount(FileList, {
    props: {
      archives: [archive()],
      entries: [],
      insideArchive: false,
      archiveId: null,
      viewMode: 'details' as const,
      iconSize: 128,
      sortMode: 'name' as const,
      sortDesc: false,
      search: '',
      loading: false,
      status: 'ok',
      selected: [],
      ...over,
    },
    attachTo: document.body,
  })
}

describe('FileList', () => {
  it('renders archives in a folder', () => {
    const w = mountList({ archives: [archive({ id: 1 }), archive({ id: 2, name: 'b.zip' })] })
    expect(w.findAll('.item')).toHaveLength(2)
  })

  it('renders entries inside an archive', () => {
    const w = mountList({
      archives: [],
      entries: [entry(0), entry(1), entry(2)],
      insideArchive: true,
      archiveId: 5,
    })
    expect(w.findAll('.item')).toHaveLength(3)
  })

  it('uses the opaque thumbnail URL for entries', () => {
    const w = mountList({
      archives: [],
      entries: [entry(3)],
      insideArchive: true,
      archiveId: 5,
    })
    const src = w.find('.item img').attributes('src') ?? ''
    expect(src).toBe('/img/t5/3/128')
    expect(src).not.toContain('C:')
    expect(src).not.toContain('file:')
  })

  it('uses the cover URL for archives', () => {
    const w = mountList()
    expect(w.find('.item img').attributes('src')).toBe('/img/c1/192')
  })

  it('shows the details columns', () => {
    const w = mountList()
    expect(w.find('.header').exists()).toBe(true)
    expect(w.text()).toContain('名前')
    expect(w.text()).toContain('サイズ')
    expect(w.text()).toContain('更新日時')
  })

  it('hides the column header in icon views', () => {
    expect(mountList({ viewMode: 'medium' }).find('.header').exists()).toBe(false)
    expect(mountList({ viewMode: 'large' }).find('.header').exists()).toBe(false)
  })

  it('renders medium and large icon layouts', () => {
    expect(mountList({ viewMode: 'medium' }).find('.item.medium').exists()).toBe(true)
    expect(mountList({ viewMode: 'large' }).find('.item.large').exists()).toBe(true)
  })

  it('shows the image count for an archive', () => {
    const w = mountList({ archives: [archive({ count: 42 })] })
    expect(w.text()).toContain('42 枚')
  })

  it('shows a status label for a broken archive', () => {
    const w = mountList({ archives: [archive({ status: 'corrupt', count: 0 })] })
    expect(w.text()).toContain('破損')
  })

  it('marks the selected items', () => {
    const w = mountList({
      archives: [archive({ id: 1, name: 'a.zip' }), archive({ id: 2, name: 'b.zip' })],
      selected: ['a:2'],
    })
    const items = w.findAll('.item')
    expect(items[0].classes()).not.toContain('selected')
    expect(items[1].classes()).toContain('selected')
  })
  it('emits select on click', async () => {
    const w = mountList()
    await w.find('.item').trigger('click')
    expect(w.emitted('select')?.[0]?.[0]).toEqual(['a:1'])
  })

  it('extends the selection with shift', async () => {
    const w = mountList({
      archives: [
        archive({ id: 1, name: 'a.zip' }),
        archive({ id: 2, name: 'b.zip' }),
        archive({ id: 3, name: 'c.zip' }),
      ],
    })
    await w.findAll('.item')[0].trigger('click')
    await w.findAll('.item')[2].trigger('click', { shiftKey: true })
    const last = w.emitted('select')?.at(-1)?.[0] as string[]
    expect(last).toHaveLength(3)
  })

  it('toggles the selection with ctrl', async () => {
    const w = mountList({
      archives: [archive({ id: 1, name: 'a.zip' }), archive({ id: 2, name: 'b.zip' })],
      selected: ['a:1'],
    })
    await w.findAll('.item')[1].trigger('click', { ctrlKey: true })
    const last = w.emitted('select')?.at(-1)?.[0] as string[]
    // The existing selection is preserved and the clicked item is added.
    expect(last).toContain('a:1')
    expect(last).toContain('a:2')
    expect(last).toHaveLength(2)
  })

  it('removes an item from the selection with ctrl', async () => {
    const w = mountList({
      archives: [archive({ id: 1, name: 'a.zip' }), archive({ id: 2, name: 'b.zip' })],
      selected: ['a:1', 'a:2'],
    })
    await w.findAll('.item')[0].trigger('click', { ctrlKey: true })
    const last = w.emitted('select')?.at(-1)?.[0] as string[]
    expect(last).toEqual(['a:2'])
  })

  it('emits open on double click', async () => {
    const w = mountList()
    await w.find('.item').trigger('dblclick')
    expect(w.emitted('open')?.[0]).toEqual(['a:1'])
  })

  it('emits context-menu with coordinates', async () => {
    const w = mountList()
    await w.find('.item').trigger('contextmenu', { clientX: 100, clientY: 200 })
    expect(w.emitted('context-menu')?.[0]?.[0]).toBe('a:1')
  })

  it('sorts by name when the name column is clicked', async () => {
    const w = mountList()
    await w.findAll('.col')[0].trigger('click')
    expect(w.emitted('update:sortDesc')?.[0]).toEqual([true])
  })

  it('switches the sort column', async () => {
    const w = mountList()
    await w.findAll('.col')[2].trigger('click')
    expect(w.emitted('update:sortMode')?.[0]).toEqual(['size'])
  })

  it('filters by the search query', () => {
    const w = mountList({
      archives: [archive({ id: 1, name: 'alpha.zip' }), archive({ id: 2, name: 'beta.zip' })],
      search: 'alpha',
    })
    expect(w.findAll('.item')).toHaveLength(1)
  })

  it('sorts naturally by name', () => {
    const w = mountList({
      archives: [archive({ id: 1, name: 'page10.zip' }), archive({ id: 2, name: 'page2.zip' })],
    })
    const names = w.findAll('.item .name').map((n) => n.text())
    expect(names).toEqual(['page2.zip', 'page10.zip'])
  })

  it('shows the empty message for an empty folder', () => {
    const w = mountList({ archives: [] })
    expect(w.text()).toContain('ZIP が見つかりません')
  })

  it('shows the empty message for an archive with no images', () => {
    const w = mountList({ archives: [], entries: [], insideArchive: true, status: 'empty' })
    expect(w.text()).toContain('画像がありません')
  })

  it('shows a loading overlay', () => {
    expect(mountList({ loading: true }).text()).toContain('読み込み中')
  })

  it('shows a placeholder for an encrypted entry', () => {
    const w = mountList({
      archives: [],
      entries: [{ ...entry(0), encrypted: true }],
      insideArchive: true,
      archiveId: 5,
    })
    expect(w.find('.item img').exists()).toBe(false)
    expect(w.text()).toContain('暗号化')
  })

  it('replaces a failed thumbnail with a placeholder', async () => {
    const w = mountList()
    await w.find('.item img').trigger('error')
    expect(w.find('.item img').exists()).toBe(false)
    expect(w.find('.placeholder').exists()).toBe(true)
  })

  it('does not render every item for a large list', () => {
    const many: ArchiveSummary[] = []
    for (let i = 0; i < 5000; i++) many.push(archive({ id: i + 1, name: `book${i}.zip` }))
    const w = mountList({ archives: many })
    expect(w.findAll('.item').length).toBeLessThan(200)
  })

  it('lays out icon views in multiple columns', () => {
    const many: ArchiveSummary[] = []
    for (let i = 0; i < 40; i++) many.push(archive({ id: i + 1, name: `b${i}.zip` }))
    const w = mountList({ archives: many, viewMode: 'medium', iconSize: 128 })
    const items = w.findAll('.item')
    const lefts = new Set(items.map((i) => (i.element as HTMLElement).style.left))
    expect(lefts.size).toBeGreaterThan(1)
  })
})
