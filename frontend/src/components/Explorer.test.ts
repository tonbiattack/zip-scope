import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AddressBar from './AddressBar.vue'
import CommandBar from './CommandBar.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

function mountBar(over: Record<string, unknown> = {}) {
  return mount(AddressBar, {
    props: {
      path: 'C:\\pics\\books',
      insideArchive: false,
      canBack: false,
      canForward: false,
      canUp: true,
      scanning: false,
      ...over,
    },
  })
}

describe('AddressBar', () => {
  it('splits the path into breadcrumb segments', () => {
    const w = mountBar()
    const crumbs = w.findAll('.crumb').map((c) => c.text())
    // The root button has no text, so the segments follow it.
    expect(crumbs).toContain('C:')
    expect(crumbs).toContain('pics')
    expect(crumbs).toContain('books')
  })

  it('marks the last segment as the current location', () => {
    const w = mountBar()
    expect(w.find('.crumb.current').text()).toBe('books')
  })

  it('shows a ZIP badge when inside an archive', () => {
    const w = mountBar({ path: 'C:\\pics\\book.zip', insideArchive: true })
    expect(w.find('.zip-badge').exists()).toBe(true)
    expect(w.find('.crumb.current').text()).toContain('book.zip')
  })

  it('disables back and forward when there is no history', () => {
    const w = mountBar()
    const buttons = w.findAll('.navbtn')
    expect(buttons[0].attributes('disabled')).toBeDefined()
    expect(buttons[1].attributes('disabled')).toBeDefined()
  })

  it('enables back when history exists', () => {
    const w = mountBar({ canBack: true })
    expect(w.findAll('.navbtn')[0].attributes('disabled')).toBeUndefined()
  })

  it('emits back, forward and up', async () => {
    const w = mountBar({ canBack: true, canForward: true })
    const buttons = w.findAll('.navbtn')
    await buttons[0].trigger('click')
    await buttons[1].trigger('click')
    await buttons[2].trigger('click')
    expect(w.emitted('back')).toBeTruthy()
    expect(w.emitted('forward')).toBeTruthy()
    expect(w.emitted('up')).toBeTruthy()
  })

  it('emits navigate with the clicked segment path', async () => {
    const w = mountBar()
    const pics = w.findAll('.crumb').find((c) => c.text() === 'pics')
    await pics?.trigger('click')
    expect(w.emitted('navigate')?.[0]).toEqual(['C:\\pics'])
  })

  it('does not emit navigate for the current segment', async () => {
    const w = mountBar()
    await w.find('.crumb.current').trigger('click')
    expect(w.emitted('navigate')).toBeFalsy()
  })

  it('emits pick-folder from the root button', async () => {
    const w = mountBar()
    await w.find('.crumb.root').trigger('click')
    expect(w.emitted('pick-folder')).toBeTruthy()
  })

  it('emits refresh', async () => {
    const w = mountBar()
    const buttons = w.findAll('.navbtn')
    await buttons[buttons.length - 1].trigger('click')
    expect(w.emitted('refresh')).toBeTruthy()
  })

  it('shows a scanning indicator', () => {
    const w = mountBar({ scanning: true })
    expect(w.text()).toContain('走査中')
  })

  it('handles an empty path', () => {
    const w = mountBar({ path: '' })
    expect(w.findAll('.crumb.current')).toHaveLength(0)
  })
})

function mountCmd(over: Record<string, unknown> = {}) {
  return mount(CommandBar, {
    props: {
      insideArchive: false,
      hasSelection: true,
      selectionCount: 1,
      viewMode: 'details' as const,
      iconSize: 128,
      scanning: false,
      canRecycle: true,
      ...over,
    },
  })
}

describe('CommandBar', () => {
  it('shows the selection count', () => {
    expect(mountCmd({ selectionCount: 1 }).text()).toContain('1 件を選択')
    expect(mountCmd({ selectionCount: 3 }).text()).toContain('3 件を選択')
    expect(mountCmd({ selectionCount: 0, hasSelection: false }).text()).toContain('未選択')
  })

  it('disables actions that need a selection', () => {
    const w = mountCmd({ hasSelection: false, selectionCount: 0 })
    const open = w.findAll('.cmd').find((b) => b.text() === '開く')
    expect(open?.attributes('disabled')).toBeDefined()
  })

  it('hides organize actions inside an archive', () => {
    const w = mountCmd({ insideArchive: true })
    const labels = w.findAll('.cmd').map((b) => b.text())
    expect(labels).not.toContain('名前変更')
    expect(labels).not.toContain('削除')
  })

  it('shows organize actions in a folder', () => {
    const w = mountCmd()
    const labels = w.findAll('.cmd').map((b) => b.text())
    expect(labels).toContain('名前変更')
    expect(labels).toContain('移動')
    expect(labels).toContain('削除')
  })

  it('disables delete when the Recycle Bin is unavailable', () => {
    const w = mountCmd({ canRecycle: false })
    const del = w.findAll('.cmd').find((b) => b.text() === '削除')
    expect(del?.attributes('disabled')).toBeDefined()
  })

  it('emits the organize actions', async () => {
    const w = mountCmd()
    for (const label of ['名前変更', '移動', '削除']) {
      const btn = w.findAll('.cmd').find((b) => b.text() === label)
      await btn?.trigger('click')
    }
    expect(w.emitted('rename')).toBeTruthy()
    expect(w.emitted('move')).toBeTruthy()
    expect(w.emitted('delete')).toBeTruthy()
  })

  it('emits open and reveal', async () => {
    const w = mountCmd()
    await w.findAll('.cmd').find((b) => b.text() === '開く')?.trigger('click')
    await w.findAll('.cmd').find((b) => b.text() === '場所を開く')?.trigger('click')
    expect(w.emitted('open')).toBeTruthy()
    expect(w.emitted('reveal')).toBeTruthy()
  })

  it('switches the view mode', async () => {
    const w = mountCmd()
    const buttons = w.findAll('.viewbtn')
    await buttons[1].trigger('click')
    expect(w.emitted('update:viewMode')?.[0]).toEqual(['medium'])
    await buttons[2].trigger('click')
    expect(w.emitted('update:viewMode')?.[1]).toEqual(['large'])
  })

  it('marks the active view mode', () => {
    const w = mountCmd({ viewMode: 'large' })
    const buttons = w.findAll('.viewbtn')
    expect(buttons[2].classes()).toContain('on')
    expect(buttons[0].classes()).not.toContain('on')
  })

  it('shows the icon slider only in icon views', () => {
    expect(mountCmd({ viewMode: 'details' }).find('.slider').exists()).toBe(false)
    expect(mountCmd({ viewMode: 'medium' }).find('.slider').exists()).toBe(true)
  })

  it('emits the icon size from the slider', async () => {
    const w = mountCmd({ viewMode: 'large' })
    await w.find('.slider').setValue('192')
    expect(w.emitted('update:iconSize')?.[0]).toEqual([192])
  })

  it('shows a cancel button while scanning', async () => {
    const w = mountCmd({ scanning: true })
    const cancel = w.findAll('.cmd').find((b) => b.text() === '中止')
    expect(cancel).toBeTruthy()
    await cancel?.trigger('click')
    expect(w.emitted('cancel-scan')).toBeTruthy()
  })

  it('emits settings', async () => {
    const w = mountCmd()
    await w.findAll('.cmd').find((b) => b.text() === '設定')?.trigger('click')
    expect(w.emitted('settings')).toBeTruthy()
  })
})
