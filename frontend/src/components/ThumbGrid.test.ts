import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ThumbGrid from './ThumbGrid.vue'
import type { EntryInfo } from '../lib/types'

function entry(ordinal: number, name = `page${ordinal}.jpg`): EntryInfo {
  return { ordinal, name, crc32: ordinal, size: 1000, encrypted: false }
}

function mountGrid(entries: EntryInfo[], over: Record<string, unknown> = {}) {
  return mount(ThumbGrid, {
    props: {
      archiveId: 1,
      entries,
      thumbSize: 192,
      selectedOrdinal: null,
      loading: false,
      status: 'ok',
      ...over,
    },
    attachTo: document.body,
  })
}

describe('ThumbGrid', () => {
  it('renders a cell per entry', () => {
    const w = mountGrid([entry(0), entry(1), entry(2)])
    expect(w.findAll('.cell')).toHaveLength(3)
  })

  it('requests thumbnails through the opaque id URL', () => {
    const w = mountGrid([entry(5)])
    const img = w.find('.cell img')
    expect(img.attributes('src')).toBe('/img/t1/5/192')
  })

  it('never puts a filesystem path in the image URL', () => {
    const w = mountGrid([entry(3)])
    const src = w.find('.cell img').attributes('src') ?? ''
    expect(src.startsWith('/img/')).toBe(true)
    expect(src).not.toContain('\\')
    expect(src).not.toContain('C:')
    expect(src).not.toContain('file:')
    expect(src).not.toContain('http')
  })

  it('emits select on click', async () => {
    const e = entry(2)
    const w = mountGrid([e])
    await w.find('.cell').trigger('click')
    expect(w.emitted('select')?.[0]?.[0]).toEqual(e)
  })

  it('emits open on double click', async () => {
    const e = entry(4)
    const w = mountGrid([e])
    await w.find('.cell').trigger('dblclick')
    expect(w.emitted('open')?.[0]?.[0]).toEqual(e)
  })

  it('marks the selected cell', () => {
    const w = mountGrid([entry(0), entry(1)], { selectedOrdinal: 1 })
    const cells = w.findAll('.cell')
    expect(cells[0].classes()).not.toContain('selected')
    expect(cells[1].classes()).toContain('selected')
  })

  it('shows a placeholder for an encrypted entry', () => {
    const w = mountGrid([{ ...entry(0), encrypted: true }])
    expect(w.find('.cell img').exists()).toBe(false)
    expect(w.text()).toContain('暗号化')
  })

  it('shows the empty message for an archive with no images', () => {
    const w = mountGrid([], { status: 'empty' })
    expect(w.text()).toContain('画像がありません')
  })

  it('shows the error message for a broken archive', () => {
    const w = mountGrid([], { status: 'corrupt', error: 'bad central directory' })
    expect(w.text()).toContain('bad central directory')
  })

  it('shows a loading overlay', () => {
    const w = mountGrid([], { loading: true })
    expect(w.text()).toContain('読み込み中')
  })

  it('prompts for a selection when no archive is open', () => {
    const w = mountGrid([], { archiveId: null })
    expect(w.text()).toContain('ZIP を選択してください')
  })

  it('emits the new thumbnail size from the slider', async () => {
    const w = mountGrid([entry(0)])
    await w.find('input[type=range]').setValue('256')
    expect(w.emitted('update:thumbSize')?.[0]).toEqual([256])
  })

  it('does not render every cell for a large archive', () => {
    const many: EntryInfo[] = []
    for (let i = 0; i < 2000; i++) many.push(entry(i))
    const w = mountGrid(many)
    expect(w.findAll('.cell').length).toBeLessThan(200)
  })

  it('shows the entry count in the header', () => {
    const w = mountGrid([entry(0), entry(1), entry(2)])
    expect(w.find('.count').text()).toBe('3 枚')
  })

  it('replaces a failed thumbnail with a placeholder', async () => {
    const w = mountGrid([entry(0)])
    await w.find('.cell img').trigger('error')
    expect(w.find('.cell img').exists()).toBe(false)
    expect(w.find('.bad').exists()).toBe(true)
  })
})
