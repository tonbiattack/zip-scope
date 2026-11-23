import { describe, expect, it } from 'vitest'
import {
  errorKindLabel,
  formatBytes,
  formatDate,
  formatDuration,
  naturalCompare,
  statusLabel,
} from './format'

describe('formatBytes', () => {
  it('formats small values as bytes', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
  })

  it('formats larger values with units', () => {
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(2 * 1024 * 1024 * 1024)).toBe('2.0 GB')
  })

  it('handles invalid input', () => {
    expect(formatBytes(-1)).toBe('—')
    expect(formatBytes(Number.NaN)).toBe('—')
  })
})

describe('formatDate', () => {
  it('formats a timestamp', () => {
    const d = new Date(2026, 0, 2, 3, 4)
    expect(formatDate(d.getTime())).toBe('2026-01-02 03:04')
  })

  it('handles zero and invalid input', () => {
    expect(formatDate(0)).toBe('—')
    expect(formatDate(Number.NaN)).toBe('—')
  })
})

describe('formatDuration', () => {
  it('uses milliseconds below one second', () => {
    expect(formatDuration(250)).toBe('250 ms')
  })

  it('uses seconds above one second', () => {
    expect(formatDuration(1500)).toBe('1.5 s')
  })
})

describe('naturalCompare', () => {
  it('orders numeric runs by value', () => {
    expect(naturalCompare('page2.jpg', 'page10.jpg')).toBeLessThan(0)
    expect(naturalCompare('page10.jpg', 'page2.jpg')).toBeGreaterThan(0)
  })

  it('treats equal names as equal', () => {
    expect(naturalCompare('a.jpg', 'a.jpg')).toBe(0)
  })

  it('is case insensitive for the primary comparison', () => {
    expect(naturalCompare('A.jpg', 'a.jpg')).toBeLessThan(0)
  })

  it('sorts a realistic page list', () => {
    const names = ['page10.jpg', 'page2.jpg', 'page1.jpg', 'page20.jpg', 'page3.jpg']
    const sorted = [...names].sort(naturalCompare)
    expect(sorted).toEqual(['page1.jpg', 'page2.jpg', 'page3.jpg', 'page10.jpg', 'page20.jpg'])
  })

  it('orders directory components before contents', () => {
    expect(naturalCompare('sub/page1.jpg', 'sub/page2.jpg')).toBeLessThan(0)
  })
})

describe('statusLabel', () => {
  it('returns an empty label for ok', () => {
    expect(statusLabel('ok')).toBe('')
  })

  it('labels the failure states', () => {
    expect(statusLabel('empty')).toBe('画像なし')
    expect(statusLabel('corrupt')).toBe('破損')
    expect(statusLabel('encrypted')).toBe('暗号化')
    expect(statusLabel('missing')).toBe('見つかりません')
    expect(statusLabel('error')).toBe('エラー')
  })
})

describe('errorKindLabel', () => {
  it('labels each error kind', () => {
    expect(errorKindLabel('encrypted')).toBe('暗号化')
    expect(errorKindLabel('corrupt')).toBe('破損')
    expect(errorKindLabel('unsupported')).toBe('非対応')
    expect(errorKindLabel('too-large')).toBe('サイズ超過')
    expect(errorKindLabel('too-many-pixels')).toBe('画素数超過')
    expect(errorKindLabel('missing')).toBe('なし')
    expect(errorKindLabel('changed')).toBe('更新あり')
    expect(errorKindLabel('whatever')).toBe('読込失敗')
  })
})
