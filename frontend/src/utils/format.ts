import dayjs from 'dayjs'

export function formatDateTime(iso: string | null | undefined): string {
  return iso ? dayjs(iso).format('YYYY-MM-DD HH:mm:ss') : '-'
}

export function formatDate(iso: string | null | undefined): string {
  return iso ? dayjs(iso).format('YYYY-MM-DD') : '-'
}

export function defaultRange(days = 7): [string, string] {
  return [dayjs().subtract(days, 'day').format('YYYY-MM-DD'), dayjs().format('YYYY-MM-DD')]
}
