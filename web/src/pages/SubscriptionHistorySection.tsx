import { Archive, Film, Play, RotateCcw } from 'lucide-react'

import { imageURL } from '../api/client'
import type { Subscription } from '../types'
import { subscriptionProgressLabel } from './subscriptionPageModel'

interface SubscriptionHistorySectionProps {
  subscriptions: Subscription[]
  onRestore: (subscription: Subscription, runAfterRestore?: boolean) => void
}

export function SubscriptionHistorySection({ subscriptions, onRestore }: SubscriptionHistorySectionProps) {
  if (subscriptions.length === 0) return null

  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2">
        <Archive size={18} className="text-brand-500" />
        <h2 className="font-display text-xl font-semibold text-ink-600">订阅历史</h2>
        <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-ink-50">{subscriptions.length} 条</span>
      </div>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {subscriptions.map((subscription) => (
          <article key={subscription.id} className="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm">
            <div className="flex gap-3">
              <div className="h-20 w-14 flex-shrink-0 overflow-hidden rounded-xl bg-primary-400/10">
                {subscription.poster_url ? (
                  <img src={imageURL(subscription.poster_url, subscription.updated_at)} alt={subscription.name} className="h-full w-full object-cover" />
                ) : (
                  <div className="flex h-full items-center justify-center text-brand-500">
                    <Film size={18} />
                  </div>
                )}
              </div>
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-semibold text-ink-600" title={subscription.name}>
                  {subscription.name}
                </h3>
                <p className="mt-1 text-xs text-ink-50">{subscription.archive_reason || '订阅已完成'}</p>
                <p className="mt-2 text-xs text-ink-100">
                  {subscription.archived_at ? new Date(subscription.archived_at).toLocaleString() : '完成时间未知'}
                </p>
                <p className="mt-1 text-xs text-ink-50">{subscriptionProgressLabel(subscription)}</p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <button
                    className="rounded-xl border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-ink-100 hover:bg-gray-50"
                    onClick={() => onRestore(subscription)}
                  >
                    <RotateCcw size={13} className="mr-1 inline" />
                    恢复订阅
                  </button>
                  <button
                    className="rounded-xl border border-primary-400/40 bg-white px-3 py-1.5 text-xs font-semibold text-brand-500 hover:bg-primary-400/10"
                    onClick={() => onRestore(subscription, true)}
                  >
                    <Play size={13} className="mr-1 inline" />
                    恢复并运行
                  </button>
                </div>
              </div>
            </div>
          </article>
        ))}
      </div>
    </section>
  )
}
