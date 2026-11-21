import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ArchiveMenu from './ArchiveMenu.vue'
import PromptDialog from './PromptDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ArchiveMenu', () => {
  function mountMenu(over: Record<string, unknown> = {}) {
    return mount(ArchiveMenu, {
      props: { x: 100, y: 100, archiveName: 'book.zip', canRecycle: true, ...over },
      attachTo: document.body,
    })
  }

  it('shows the archive name', () => {
    const w = mountMenu()
    expect(w.find('.head').text()).toBe('book.zip')
  })

  it('offers the four actions', () => {
    const w = mountMenu()
    const labels = w.findAll('.item').map((b) => b.text())
    expect(labels).toEqual([
      'エクスプローラで開く',
      '名前を変更…',
      '別のフォルダへ移動…',
      'ごみ箱へ移動',
    ])
  })

  it('emits reveal', async () => {
    const w = mountMenu()
    await w.findAll('.item')[0].trigger('click')
    expect(w.emitted('reveal')).toBeTruthy()
    expect(w.emitted('close')).toBeTruthy()
  })

  it('emits rename', async () => {
    const w = mountMenu()
    await w.findAll('.item')[1].trigger('click')
    expect(w.emitted('rename')).toBeTruthy()
  })

  it('emits move', async () => {
    const w = mountMenu()
    await w.findAll('.item')[2].trigger('click')
    expect(w.emitted('move')).toBeTruthy()
  })

  it('emits delete', async () => {
    const w = mountMenu()
    await w.findAll('.item')[3].trigger('click')
    expect(w.emitted('delete')).toBeTruthy()
  })

  it('disables the delete action when the Recycle Bin is unavailable', () => {
    const w = mountMenu({ canRecycle: false })
    const del = w.findAll('.item')[3]
    expect(del.attributes('disabled')).toBeDefined()
  })

  it('closes on Escape', async () => {
    const w = mountMenu()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()
    expect(w.emitted('close')).toBeTruthy()
  })

  it('closes when clicking outside', async () => {
    const w = mountMenu()
    const outside = document.createElement('div')
    document.body.appendChild(outside)
    // jsdom has no PointerEvent, so a plain Event with the same type is used;
    // the listener only cares about the event type and target.
    outside.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.emitted('close')).toBeTruthy()
  })

  it('keeps the menu inside the window', async () => {
    const w = mountMenu({ x: 99999, y: 99999 })
    await w.vm.$nextTick()
    const style = w.find('.menu').attributes('style') ?? ''
    const left = Number(/left:\s*(\d+)px/.exec(style)?.[1] ?? '0')
    expect(left).toBeLessThan(window.innerWidth)
  })
})

describe('PromptDialog', () => {
  function mountDialog(over: Record<string, unknown> = {}) {
    return mount(PromptDialog, {
      props: { title: 'Test', ...over },
      attachTo: document.body,
    })
  }

  it('shows the title and message', () => {
    const w = mountDialog({ message: 'Are you sure?' })
    expect(w.find('h2').text()).toBe('Test')
    expect(w.text()).toContain('Are you sure?')
  })

  it('shows a text field only in prompt mode', () => {
    expect(mountDialog().find('.field').exists()).toBe(false)
    expect(mountDialog({ prompt: true }).find('.field').exists()).toBe(true)
  })

  it('prefills the field with the current value', () => {
    const w = mountDialog({ prompt: true, value: 'book.zip' })
    expect((w.find('.field').element as HTMLInputElement).value).toBe('book.zip')
  })

  it('emits confirm with the typed value', async () => {
    const w = mountDialog({ prompt: true, value: 'old.zip' })
    await w.find('.field').setValue('new.zip')
    await w.findAll('.actions .btn')[1].trigger('click')
    expect(w.emitted('confirm')?.[0]).toEqual(['new.zip'])
  })

  it('refuses an empty name', async () => {
    const w = mountDialog({ prompt: true, value: 'x.zip' })
    await w.find('.field').setValue('   ')
    await w.findAll('.actions .btn')[1].trigger('click')
    expect(w.emitted('confirm')).toBeFalsy()
    expect(w.find('.error').exists()).toBe(true)
  })

  it('emits cancel from the button', async () => {
    const w = mountDialog()
    await w.findAll('.actions .btn')[0].trigger('click')
    expect(w.emitted('cancel')).toBeTruthy()
  })

  it('emits cancel on Escape', async () => {
    const w = mountDialog()
    await w.find('.dialog').trigger('keydown', { key: 'Escape' })
    expect(w.emitted('cancel')).toBeTruthy()
  })

  it('confirms on Enter', async () => {
    const w = mountDialog({ prompt: true, value: 'a.zip' })
    await w.find('.field').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('confirm')?.[0]).toEqual(['a.zip'])
  })

  it('uses the danger style for destructive actions', () => {
    const w = mountDialog({ danger: true, confirmLabel: '削除' })
    const confirm = w.findAll('.actions .btn')[1]
    expect(confirm.classes()).toContain('danger')
    expect(confirm.text()).toBe('削除')
  })
})
