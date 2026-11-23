/*
 * Mailbox UI smoke tests. They drive the real app with a mocked API so the
 * loading, empty, error, detail, mobile and auth-guard paths stay honest.
 */
import assert from 'node:assert/strict'
import {existsSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import {after, before, test} from 'node:test'
import {createServer} from 'vite'
import puppeteer from 'puppeteer-core'

let server, browser, baseURL

before(async () => {
  const executablePath = process.env.CHROME_BIN || [
    '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  ].find(existsSync)
  assert.ok(executablePath, 'Install Chrome/Chromium or set CHROME_BIN to its executable')
  server = await createServer({
    root: fileURLToPath(new URL('../', import.meta.url)),
    server: {host: '127.0.0.1', port: 0},
    logLevel: 'error',
  })
  await server.listen()
  baseURL = `http://127.0.0.1:${server.httpServer.address().port}`
  const args = ['--disable-dev-shm-usage']
  if (process.getuid?.() === 0) args.push('--no-sandbox')
  browser = await puppeteer.launch({executablePath, headless: true, args})
})

after(async () => {
  await browser?.close()
  await server?.close()
})

const SESSION = {errorNo: 0, data: {account: 'neo', name: 'Neo', domain: 'example.com', domains: ['example.com'], is_admin: true}}
const NO_SESSION = {errorNo: 403, errorMsg: 'login required', data: null}
const GROUPS = {
  errorNo: 0,
  data: [
    {id: 0, label: 'Inbox', tag: '{"type":0,"status":-1,"group_id":0}', unread: 3, children: []},
    {id: 0, label: 'Sent', tag: '{"type":1,"status":-1,"group_id":0}', unread: 0, children: []},
    {id: 8, label: 'Work', tag: '{"group_id":8,"status":-1,"type":-1}', unread: 5, children: []},
  ],
}
const ITEMS = [
  {id: 2, title: 'Quarterly report', desc: 'Attached numbers', datetime: new Date().toISOString(), is_read: false, sender: {Name: 'Ada', EmailAddress: 'ada@example.org'}, to: [], dangerous: false, error: ''},
  {id: 1, title: '', desc: 'no subject', datetime: '2024-02-03T10:00:00Z', is_read: true, sender: {Name: 'Bob', EmailAddress: 'bob@example.net'}, to: [], dangerous: false, error: ''},
]

async function openApp(options = {}) {
  const page = await browser.newPage()
  if (options.mobile) {
    await page.setViewport({width: 390, height: 844, isMobile: true, hasTouch: true, deviceScaleFactor: 2})
  } else {
    await page.setViewport({width: 1280, height: 800})
  }
  await page.setRequestInterception(true)
  page.on('request', (request) => {
    const url = new URL(request.url())
    const json = (body) => request.respond({status: 200, contentType: 'application/json', body: JSON.stringify(body)})
    if (!url.pathname.startsWith('/api/')) return request.continue()
    if (url.pathname === '/api/user/info') return json(options.userReply || SESSION)
    if (url.pathname === '/api/group' || url.pathname === '/api/group/list') return json(GROUPS)
    if (url.pathname === '/api/logout') return json({errorNo: 0, data: null})
    if (url.pathname === '/api/email/list') {
      return json((options.listReply || (() => ({errorNo: 0, data: {current_page: 1, total_page: 1, list: ITEMS}})))())
    }
    if (url.pathname === '/api/email/detail') {
      return json(options.detailReply || {errorNo: 0, data: {}})
    }
    return json({errorNo: 0, data: {}})
  })
  return page
}

const visible = (page, selector) => page.evaluate((sel) => {
  const el = document.querySelector(sel)
  if (!el) return false
  const rect = el.getBoundingClientRect()
  const style = window.getComputedStyle(el)
  return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none'
}, selector)

test('an empty folder waits with a skeleton, then explains itself', async (t) => {
  const page = await openApp({listReply: () => ({errorNo: 0, data: {current_page: 1, total_page: 0, list: []}})})
  t.after(() => page.close())
  await page.goto(`${baseURL}/#/`, {waitUntil: 'domcontentloaded'})
  await page.waitForSelector('.skeleton-list')
  await page.waitForSelector('.list-state')
  assert.match(await page.$eval('.list-state', el => el.innerText), /No messages here yet/)
})

test('rows, unread badges and the document title come from the API', async (t) => {
  const page = await openApp()
  t.after(() => page.close())
  await page.goto(`${baseURL}/#/`, {waitUntil: 'domcontentloaded'})
  await page.waitForSelector('.mail-row-content')
  const rows = await page.$$eval('.mail-row-content', els => els.map(el => el.innerText))
  assert.equal(rows.length, 2)
  assert.match(rows[0], /Ada/)
  assert.match(rows[0], /Quarterly report/)
  assert.match(rows[1], /\(no subject\)/)
  await page.waitForFunction(() => document.querySelectorAll('.menu-badge').length > 0)
  const badges = await page.$$eval('.menu-badge', els => els.map(el => el.textContent.trim()))
  assert.ok(badges.includes('3') && badges.includes('5'), `badges were ${badges}`)
  assert.equal(await page.title(), '(3) iMail')
})

test('a failed list request offers a retry instead of a blank pane', async (t) => {
  const page = await openApp({listReply: () => ({errorNo: 500, errorMsg: 'database is on fire'})})
  t.after(() => page.close())
  await page.goto(`${baseURL}/#/`, {waitUntil: 'domcontentloaded'})
  await page.waitForSelector('.list-state')
  assert.match(await page.$eval('.list-state', el => el.innerText), /database is on fire/)
  assert.ok(await visible(page, '.state-action'))
})

test('a message renders and remote images wait for permission', async (t) => {
  const detail = {
    errorNo: 0,
    data: {
      id: 2, subject: 'Quarterly report', from_name: 'Ada', from_address: 'ada@example.org',
      to: JSON.stringify([{Name: 'Neo', EmailAddress: 'neo@example.com'}]), cc: '[]',
      text: 'plain body', html: '<p>rich body</p><img src="https://tracker.example/p.gif">',
      attachments: [], send_date: '2025-01-02T03:04:05Z', status: 0, type: 0,
    },
  }
  const page = await openApp({detailReply: detail})
  t.after(() => page.close())
  await page.goto(`${baseURL}/#/detail/2`, {waitUntil: 'domcontentloaded'})
  await page.waitForSelector('.mail-body-frame')
  assert.match(await page.$eval('.mail-meta-card', el => el.innerText), /Ada/)
  await page.waitForSelector('.image-gate')
  const blocked = await page.$eval('iframe', el => el.srcdoc.includes('tracker.example'))
  assert.equal(blocked, false, 'the tracking URL survived into the frame')
  await page.evaluate(() => document.querySelector('.image-gate-action').click())
  await page.waitForFunction(() => !document.querySelector('.image-gate'))
})

test('mobile swaps compose for a bottom bulk bar when a row is selected', async (t) => {
  const page = await openApp({mobile: true})
  t.after(() => page.close())
  await page.goto(`${baseURL}/#/`, {waitUntil: 'domcontentloaded'})
  await page.waitForSelector('.compose-fab')
  assert.ok(await visible(page, '.compose-fab'))
  assert.equal(await visible(page, '.compose-btn'), false)
  await page.click('.selection-col .el-checkbox')
  await page.waitForSelector('.mobile-bulk-bar')
  assert.ok(await visible(page, '.mobile-bulk-bar'))
  assert.equal(await visible(page, '.compose-fab'), false)
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  assert.ok(overflow <= 1, `horizontal overflow: ${overflow}px`)
})

test('deep links resolve the session first and unknown paths show a 404', async (t) => {
  const signedOut = await openApp({userReply: NO_SESSION})
  t.after(() => signedOut.close())
  await signedOut.goto(`${baseURL}/#/detail/9`, {waitUntil: 'domcontentloaded'})
  await signedOut.waitForFunction(() => location.hash.startsWith('#/login'))

  const page = await openApp()
  t.after(() => page.close())
  await page.goto(`${baseURL}/#/nope`, {waitUntil: 'domcontentloaded'})
  await page.waitForSelector('.not-found-card')
})

test('conversation view groups replies from different participants', async (t) => {
  const thread = [
    {id: 4, title: 'Q4 revenue review', desc: 'numbers', datetime: new Date().toISOString(), is_read: true, sender: {Name: 'Ada', EmailAddress: 'ada@example.org'}, to: [], dangerous: false, error: ''},
    {id: 5, title: 'Re: Q4 revenue review', desc: 'looks right', datetime: new Date().toISOString(), is_read: false, sender: {Name: 'Grace', EmailAddress: 'grace@example.org'}, to: [], dangerous: false, error: ''},
  ]
  const page = await openApp({listReply: () => ({errorNo: 0, data: {current_page: 1, total_page: 1, list: thread}})})
  t.after(() => page.close())
  await page.goto(`${baseURL}/#/`, {waitUntil: 'domcontentloaded'})
  await page.waitForSelector('.mail-row-content')
  const rows = await page.$$eval('.mail-row-content', els => els.length)
  assert.equal(rows, 1, 'a reply from another participant should join the conversation')
  const badge = await page.$eval('.thread-count', el => el.textContent.trim())
  assert.equal(badge, '2')
})
