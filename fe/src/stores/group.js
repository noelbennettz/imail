import { ref } from 'vue'
import { defineStore } from 'pinia'
import lang from '../i18n/i18n';
import {http} from '@/utils/axios'

const useGroupStore = defineStore('group', () => {
  const tag = ref("")
  const name = ref(lang.inbox)
  const items = ref([])
  const itemsLoaded = ref(false)
  let pending = null

  // The desktop sidebar and the mobile drawer render the same component. One
  // cached request keeps them from fetching the folder list twice on load.
  const loadItems = () => {
    if (itemsLoaded.value) return Promise.resolve(items.value)
    if (pending) return pending
    pending = http.get('/api/group', {silent: true}).then(res => {
      if (res.errorNo === 0) {
        items.value = res.data || []
        itemsLoaded.value = true
      }
      return items.value
    }).catch(() => items.value).finally(() => {
      pending = null
    })
    return pending
  }

  const refreshItems = () => {
    itemsLoaded.value = false
    return loadItems()
  }

  // Signing out must not leave another account's folder badges cached.
  const reset = () => {
    tag.value = ''
    name.value = lang.inbox
    items.value = []
    itemsLoaded.value = false
  }

  return { tag, name, items, itemsLoaded, loadItems, refreshItems, reset }
})

export default useGroupStore
