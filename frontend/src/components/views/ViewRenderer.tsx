import { CompareView } from './CompareView'
import { DistributionView } from './DistributionView'
import { ProfileView, SimilarView } from './PersonalViews'
import { RankView, RiskView } from './RiskViews'
import { useSettings } from '@/lib/store'
import type { View } from '@/lib/types'

/** Draws an assistant evidence card with the same views the other tabs use. */
export function ViewRenderer({ view }: { view: View }) {
  const values = useSettings((s) => s.values)
  switch (view.kind) {
    case 'distribution': {
      const you = values[view.request.column]
      return <DistributionView request={view.request} you={you} compact />
    }
    case 'compare': {
      const you = values[view.request.metric]
      return <CompareView request={view.request} you={typeof you === 'number' ? you : undefined} />
    }
    case 'risk':
      return <RiskView request={view.request} />
    case 'rank':
      return <RankView request={view.request} limit={8} />
    case 'profile':
      return <ProfileView request={view.request} />
    case 'similar':
      return <SimilarView request={view.request} />
  }
}
