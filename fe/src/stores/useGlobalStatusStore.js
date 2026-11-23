import {defineStore} from "pinia";
import {http} from "@/utils/axios";

// Several views ask who is signed in while the first navigation is still
// resolving. Sharing one in-flight request prevents a burst of /api/user/info
// calls and gives the router guard something to await.
let pendingInit = null

const useGlobalStatusStore = defineStore('useGlobalStatusStore', {
    state() {
        return {
            userInfos: {},
            mobileDrawerVisible: false
        }
    },
    getters: {
        isLogin(state) {
            return Object.keys(state.userInfos).length !== 0
        }
    },
    actions: {
        init(callback) {
            const done = typeof callback === 'function' ? callback : () => {}
            if (!pendingInit) {
                pendingInit = http.post("/api/user/info", {}, {silent: true}).then(res => {
                    if (res.errorNo === 0) {
                        Object.assign(this.userInfos, res.data)
                    }
                    return this.isLogin
                }).catch(() => false).finally(() => {
                    pendingInit = null
                })
            }
            return pendingInit.then(isLogin => {
                done()
                return isLogin
            })
        },
        reset() {
            this.userInfos = {}
            this.mobileDrawerVisible = false
        }
    }
})


export {useGlobalStatusStore};
