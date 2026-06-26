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

export const defaultSubscriptionExcludeWords =
  'cam,ts,tc,枪版,dovi,dv,dolby vision,dolby,杜比视界,杜比,h265,h.265,h-265,h_265,hevc,x265,10bit,10-bit,hi10p,atmos,truehd,ddp,dd+,eac3'

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
  excludeWords: defaultSubscriptionExcludeWords,
  washEnabled: false,
  washPriority: 'balanced',
}
