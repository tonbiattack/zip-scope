import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ImageViewer from './ImageViewer.vue'
import type { EntryInfo } from '../lib/types'

function entry(ordinal: number): EntryInfo {
  return { ordinal, name: `page${ordinal}.jpg`, crc32: ordinal, size: 1000, encrypted: false }
}

function entries(n: number): EntryInfo[] {
  return Array.from({ length: n }, (_, i) => entry(i))
}

function mountViewer(count = 5, index = 0) {
  return mount(ImageViewer, {
    props: {
      archiveId: 3,
      archiveName: 'book.zip',
      entries: entries(count),
      index,
    },
    attachTo: document.body,
  })
}

function press(key: string, opts: KeyboardEventInit = {}) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...opts }))
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ImageViewer', () => {
  it('requests the full image through the opaque id URL', () => {
    const w = mountViewer(5, 2)
    expect(w.find('.stage img').attributes('src')).toBe('/img/f3/2')
  })

  it('never puts a filesystem path in the image URL', () => {
    const w = mountViewer(5, 1)
    const src = w.find('.stage img').attributes('src') ?? ''
    expect(src.startsWith('/img/')).toBe(true)
    expect(src).not.toContain('C:')
    expect(src).not.toContain('file:')
  })

  it('shows the position', () => {
    const w = mountViewer(5, 2)
    expect(w.find('.pos').text()).toBe('3 / 5')
  })

  it('emits close on Escape', async () => {
    const w = mountViewer()
    press('Escape')
    await w.vm.$nextTick()
    expect(w.emitted('close')).toBeTruthy()
  })

  it('emits navigate on the right arrow', async () => {
    const w = mountViewer(5, 1)
    press('ArrowRight')
    await w.vm.$nextTick()
    expect(w.emitted('navigate')?.[0]).toEqual([2])
  })

  it('emits navigate on the left arrow', async () => {
    const w = mountViewer(5, 2)
    press('ArrowLeft')
    await w.vm.$nextTick()
    expect(w.emitted('navigate')?.[0]).toEqual([1])
  })

  it('does not navigate past the first image', async () => {
    const w = mountViewer(5, 0)
    press('ArrowLeft')
    await w.vm.$nextTick()
    expect(w.emitted('navigate')).toBeFalsy()
  })

  it('does not navigate past the last image', async () => {
    const w = mountViewer(5, 4)
    press('ArrowRight')
    await w.vm.$nextTick()
    expect(w.emitted('navigate')).toBeFalsy()
  })

  it('disables the previous button on the first image', () => {
    const w = mountViewer(5, 0)
    const buttons = w.findAll('.right .btn')
    // The first two buttons after the position are prev/next.
    const prev = buttons.find((b) => b.text() === '←')
    expect(prev?.attributes('disabled')).toBeDefined()
  })

  it('disables the next button on the last image', () => {
    const w = mountViewer(5, 4)
    const next = w.findAll('.right .btn').find((b) => b.text() === '→')
    expect(next?.attributes('disabled')).toBeDefined()
  })

  it('emits navigate-archive on Ctrl+Right', async () => {
    const w = mountViewer(5, 2)
    press('ArrowRight', { ctrlKey: true })
    await w.vm.$nextTick()
    expect(w.emitted('navigate-archive')?.[0]).toEqual([1])
  })

  it('emits navigate-archive on Ctrl+Left', async () => {
    const w = mountViewer(5, 2)
    press('ArrowLeft', { ctrlKey: true })
    await w.vm.$nextTick()
    expect(w.emitted('navigate-archive')?.[0]).toEqual([-1])
  })

  it('jumps to the first and last image with Home and End', async () => {
    const w = mountViewer(10, 5)
    press('Home')
    await w.vm.$nextTick()
    expect(w.emitted('navigate')?.[0]).toEqual([0])

    press('End')
    await w.vm.$nextTick()
    expect(w.emitted('navigate')?.[1]).toEqual([9])
  })

  it('starts in fit mode', () => {
    const w = mountViewer()
    expect(w.find('.btn.on').text()).toBe('フィット')
  })

  it('toggles between fit and 1:1', async () => {
    const w = mountViewer()
    const fit = w.findAll('.right .btn').find((b) => b.text() === 'フィット')
    await fit?.trigger('click')
    expect(w.find('.btn.on').text()).toBe('等倍')
  })
  it('zooms with the + and - keys', async () => {
    const w = mountViewer()
    // Leave fit mode first so the zoom value is observable.
    press('1')
    await w.vm.$nextTick()
    const before = w.find('.zoom').text()
    press('+')
    await w.vm.$nextTick()
    expect(w.find('.zoom').text()).not.toBe(before)
  })

  it('resets the view with 0', async () => {
    const w = mountViewer()
    press('1')
    await w.vm.$nextTick()
    press('0')
    await w.vm.$nextTick()
    expect(w.find('.btn.on').text()).toBe('フィット')
  })

  it('shows a placeholder when the image fails to load', async () => {
    const w = mountViewer()
    await w.find('.stage img').trigger('error')
    expect(w.find('.hint.bad').exists()).toBe(true)
  })

  it('removes the key listener when unmounted', async () => {
    const w = mountViewer()
    w.unmount()
    press('Escape')
    // No error and no emission after unmount.
    expect(w.emitted('close')).toBeFalsy()
  })

  it('resets the view when the image changes', async () => {
    const w = mountViewer(5, 0)
    press('1')
    await w.vm.$nextTick()
    expect(w.find('.btn.on').text()).toBe('等倍')

    await w.setProps({ index: 1 })
    expect(w.find('.btn.on').text()).toBe('フィット')
  })

  it('shows the entry name', () => {
    const w = mountViewer(5, 2)
    expect(w.find('.name').text()).toBe('page2.jpg')
  })
})
