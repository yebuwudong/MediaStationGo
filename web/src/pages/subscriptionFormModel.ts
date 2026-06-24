export interface SubscriptionFormValues {
  name: string
  feed: string
  filter: string
  mediaType: string
  mediaCategory: string
  savePath: string
  searchMode: string
  imdbID: string
  resolution: string
  quality: string
  effects: string
  releaseGroups: string
  excludeWords: string
  washEnabled: boolean
  washPriority: string
}

export const defaultSubscriptionFormValues: SubscriptionFormValues = {
  name: '',
  feed: '',
  filter: '',
  mediaType: '',
  mediaCategory: '',
  savePath: '',
  searchMode: 'keyword',
  imdbID: '',
  resolution: 'best',
  quality: '',
  effects: '',
  releaseGroups: '',
  excludeWords: 'cam,ts,tc,枪版',
  washEnabled: false,
  washPriority: 'balanced',
}
