import { describe, expect, it } from 'vitest'

import { formatUsd, isUnlimitedQuota, microToUsd, quotaText, usdToMicro } from './quota'

describe('formatUsd', () => {
  it('换算微美元为美元', () => {
    expect(formatUsd(1_000_000)).toBe('$1.00')
    expect(formatUsd(100_000_000)).toBe('$100.00')
  })

  it('小额费用保留 6 位精度', () => {
    expect(formatUsd(6789)).toBe('$0.006789')
  })

  it('达到 1 美分后按 2 位显示', () => {
    expect(formatUsd(12_345)).toBe('$0.01')
  })

  it('零值为 $0.00', () => {
    expect(formatUsd(0)).toBe('$0.00')
  })
})

describe('isUnlimitedQuota', () => {
  it('0 与 -1 均为不限额', () => {
    expect(isUnlimitedQuota(0)).toBe(true)
    expect(isUnlimitedQuota(-1)).toBe(true)
    expect(isUnlimitedQuota(1)).toBe(false)
  })
})

describe('quotaText', () => {
  it('不限额显示文案', () => {
    expect(quotaText(0)).toBe('不限额')
    expect(quotaText(-1)).toBe('不限额')
  })

  it('有额度显示金额', () => {
    expect(quotaText(100_000_000)).toBe('$100.00')
  })
})

describe('usdToMicro / microToUsd', () => {
  it('互逆换算', () => {
    expect(usdToMicro(2.5)).toBe(2_500_000)
    expect(microToUsd(2_500_000)).toBe(2.5)
    expect(usdToMicro(microToUsd(123_456_789))).toBe(123_456_789)
  })
})
