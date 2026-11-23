import {ElMessageBox} from 'element-plus'
import lang from '@/i18n/i18n'

// One confirmation dialog for the whole client: same title, localized buttons
// and a resolved boolean instead of a rejected promise that every caller has to
// swallow.
export async function confirmAction(message, options = {}) {
    try {
        await ElMessageBox.confirm(message, options.title || lang.confirm_title, {
            confirmButtonText: options.confirmText || lang.ok,
            cancelButtonText: options.cancelText || lang.cancel,
            type: options.type || 'warning',
            distinguishCancelAndClose: true,
            autofocus: false,
        })
        return true
    } catch {
        return false
    }
}
