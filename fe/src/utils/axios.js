import axios from 'axios'
import lang from '../i18n/i18n';
import {useGlobalStatusStore} from "@/stores/useGlobalStatusStore";
import {router} from "@/router";

// Create an axios instance
const http = axios.create({
    baseURL: import.meta.env.VITE_APP_URL, // base url shared by all APIs
    timeout: 60000, // request timeout
    headers: {
        'Content-Type': 'application/json;charset=UTF-8;',
        'Lang': lang.lang
    }
});

// Request interceptor
http.interceptors.request.use((config) => {
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
    if (response.data.errorNo === 403) {

        await router.replace({
            path: '/login',
            query: {
                redirect: router.currentRoute.fullPath
            }
        });
    }
    // Successful response
    if (response.data.errorNo === 402) {
        await router.replace({
            path: '/setup',
            query: {
                redirect: router.currentRoute.fullPath
            }
        });
    }
    return response.data;
}, async (error) => {
    // Response error
    if (error.response && error.response.status) {
        let message = ""
        switch (error.response.status) {
            case 400:
                message = 'Request error';
                break;
            case 401:
                message = 'Request error';
                break;
            case 403:
               await router.replace({
                    path: '/login',
                    query: {
                        redirect: router.currentRoute.fullPath
                    }
                });
                break;
            case 404:
                message = 'Requested address error';
                break;
            case 408:
                message = 'Request timed out';
                break;
            case 500:
                message = 'Internal server error!';
                break;
            case 501:
                message = 'Service not implemented!';
                break;
            case 502:
                message = 'Gateway error!';
                break;
            case 503:
                message = 'Service unavailable!';
                break;
            case 504:
                message = 'Gateway timed out!';
                break;
            case 505:
                message = 'HTTP version not supported';
                break;
            default:
                // eslint-disable-next-line no-unused-vars
                message = 'Request failed';
        }
        return Promise.reject(error);
    }
    return Promise.reject(error);
});

export {http};