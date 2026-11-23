import axios from 'axios'
import {ElMessage} from 'element-plus'
import lang from '../i18n/i18n';
import {router} from "@/router";
import {getSetupToken, isSetupAPI, SETUP_TOKEN_HEADER} from "@/utils/setupToken";

// Create an axios instance
const http = axios.create({
    baseURL: import.meta.env.VITE_APP_URL, // base url shared by all APIs
    timeout: 60000, // request timeout
    headers: {
        'Content-Type': 'application/json;charset=UTF-8;',
        'Lang': lang.lang
    }
});

// Multiple requests usually fail together when the network drops. One notice is
// enough; the rest would only stack on top of it.
let lastNoticeAt = 0

function notify(message) {
    const now = Date.now()
    if (now - lastNoticeAt < 1500) return
    lastNoticeAt = now
    ElMessage.error(message)
}

// Turns a transport failure into a sentence a person can act on. Business
// failures (`errorNo !== 0`) keep using the server's own message.
export function errorMessage(error) {
    if (error?.code === 'ECONNABORTED' || error?.code === 'ETIMEDOUT') {
        return lang.timeout_error
    }
    const status = error?.response?.status
    if (!status) return lang.network_error
    if (status === 429) return lang.too_many_attempts
    if (status >= 500) return lang.server_error
    return lang.request_failed
}

// Request interceptor
http.interceptors.request.use((config) => {
    // Every setup wizard call carries the one-time setup token once the first
    // screen has accepted it.
    if (isSetupAPI(config.url) && getSetupToken() !== '') {
        config.headers = config.headers || {}
        config.headers[SETUP_TOKEN_HEADER] = getSetupToken()
    }
    // For POST requests, convert the data payload to a JSON string
    if (config.method === 'POST') {
        config.data = JSON.stringify(config.data);
    }
    return config;
}, (error) =>
    // Handle request errors
    Promise.reject(error));

// Response interceptor
http.interceptors.response.use(async (response) => {
    // Successful response
    if (response.data.errorNo === 403 && !isSetupAPI(response.config?.url)) {

        await router.replace({
            path: '/login',
            query: {
                redirect: router.currentRoute.value.fullPath
            }
        });
    }
    // Successful response
    if (response.data.errorNo === 402) {
        await router.replace({
            path: '/setup',
            query: {
                redirect: router.currentRoute.value.fullPath
            }
        });
    }
    return response.data;
}, async (error) => {
    // Response error
    const status = error.response && error.response.status
    if (status === 403 && !isSetupAPI(error.config?.url)) {
        await router.replace({
            path: '/login',
            query: {
                redirect: router.currentRoute.value.fullPath
            }
        });
    }
    const message = errorMessage(error)
    error.userMessage = message
    if (!error.config?.silent) {
        notify(message)
    }
    return Promise.reject(error);
});

export {http};
