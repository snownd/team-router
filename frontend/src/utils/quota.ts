const MICRO = 1_000_000

// quota=0 与 quota_remaining=-1 均表示不限额（docs/api/README.md）
export function isUnlimitedQuota(value: number): boolean {
  return value === 0 || value === -1
}

export function formatUsd(microUsd: number): string {
  const dollars = microUsd / MICRO
  const digits = dollars !== 0 && Math.abs(dollars) < 0.01 ? 6 : 2
  return `$${dollars.toLocaleString('en-US', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })}`
}

export function quotaText(value: number): string {
  return isUnlimitedQuota(value) ? '不限额' : formatUsd(value)
}

export function usdToMicro(usd: number): number {
  return Math.round(usd * MICRO)
}

export function microToUsd(micro: number): number {
  return micro / MICRO
}
