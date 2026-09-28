const $ = id => document.getElementById(id)
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
const when = s => s ? new Date(s).getTime() : NaN
const ago = s => {
  const t = when(s); if (isNaN(t)) return ''
  const m = Math.round((Date.now() - t) / 60000)
  return m < 1 ? 'just now' : m < 60 ? m + 'm ago' : m < 48 * 60 ? Math.round(m / 60) + 'h ago' : Math.round(m / 1440) + 'd ago'
}
const isYou = by => by === 'you'
const byName = by => !by ? 'someone' : by
const initial = by => isYou(by) ? 'Y' : (by || '?')[0].toUpperCase()
const whoHTML = by => !by ? '<span class="who"></span>' : `<span class="who"><i class="${isYou(by) ? 'you' : /codex/i.test(by) ? 'codex' : ''}">${esc(initial(by))}</i>${esc(by)}</span>`
const md = s => esc(s).replace(/`([^`]+)`/g, '<code>$1</code>').replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>').replace(/\s*\*?\(recommended\)\*?/gi, '')
const optionLine = l => /^\s*[-*+]\s+/.test(l)

const TABS = [
  { id: 'backlog', label: 'Backlog' },
  { id: 'foryou', label: 'For you' },
  { id: 'waiting', label: 'Waiting on you' },
  { id: 'now', label: 'Right now' },
  { id: 'map', label: 'Map' },
  { id: 'quiet', label: 'Gone quiet' },
  { id: 'recent', label: 'Recently done' },
  { id: 'branches', label: 'Branches' },
]
const OLDTABS = { requests: 'foryou', questions: 'foryou' }
const NOUN = { backlog: 'task', requests: 'request', questions: 'question' }
const TASKY = k => k !== 'requests' && k !== 'questions'
const ownLists = () => (shown?.lists || []).filter(l => l.own)
const isListTab = t => t === 'backlog' || ownLists().some(l => l.kind === t)
const listTitle = k => k === 'backlog' ? 'Backlog' : k[0].toUpperCase() + k.slice(1).replace(/-/g, ' ')
const PART = { backlog: 'Tasks', requests: 'Requests', questions: 'Questions' }
const HINTS = {
  backlog: 'The work agents do, in <code>backlog.md</code>. Click a task to open it: every property, its notes and its history. Double-click a title to rename it. Press / to search, j and k to move, x to tick. Click a field to change it. <b>+ New view</b> lays the list out as a board or table by any field; views are shared with agents.',
  foryou: 'What only you can do. Questions are decisions that are yours, in <code>questions.md</code>: pick an option or answer in your own words, and the answer frees what waits on it on every branch. Requests are things like keys, accounts and deploys, in <code>requests.md</code>: tick one when it\'s done, and agents waiting on it hear about it.',
  waiting: 'Your open requests and questions, the one that frees the most work first. The count takes in everything further down the chain, not only what waits on it directly.',
  now: 'Who is working on what, by agent and branch; the tasks that are ready and that nobody has claimed, the most important first; and questions an agent answered for you so it could go on, which still need your OK.',
  quiet: 'Open items nobody has touched in a week or more: not changed, claimed, answered or ticked. The longest untouched come first.',
  recent: 'What was ticked or answered, day by day, from the dates Callboard keeps. History only.',
  map: 'What waits on what, across all the lists. Each column waits on the ones to its left, so start on the left. Point at a card to light up what it waits on and what it frees; click it to open it. Agents link items with <code>needs</code>.',
  branches: 'Each branch against the main branch, in the lists only: where it forked, what reached the main branch\'s lists since, what the branch changes, and which items would conflict if it merged now. Nothing is merged to find out. Uncommitted lists in a worktree count.',
}

let board = null, shown = null, branchesPage = null
let [tab, peekKey] = (location.hash.slice(1) || 'backlog').split('/'), branchView = null
let peekLog = null, sel = peekKey || null, query = ''
if (OLDTABS[tab]) { tab = OLDTABS[tab]; history.replaceState(null, '', '#' + tab) }
let editList = 'backlog'
let newListing = false
let listEdit = null
let railOpen = false, showHint = localStorage.getItem('cb-hint') === '1'
let seen = new Map(), fresh = new Set(), editing = null
const drafts = new Map()

async function api(path, opts = {}) {
  const r = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts })
  const body = await r.json().catch(() => ({}))
  if (!r.ok) { const e = new Error(body.error || r.statusText); e.status = r.status; throw e }
  return body
}
const send = (method, path, body) => api(path, { method, body: JSON.stringify(body) })

function toast(msg, undo) {
  document.querySelector('.toast')?.remove()
  const t = document.createElement('div'); t.className = 'toast'; t.textContent = msg; t.setAttribute('role', 'status')
  if (undo) { const b = document.createElement('button'); b.textContent = 'Undo'; b.onclick = () => { t.remove(); undo() }; t.append(b) }
  document.body.appendChild(t); setTimeout(() => t.remove(), undo ? 9000 : 4500)
}
function failed(what, err) {
  toast(err.status === 409 ? `${what}: it changed in the meantime, so the page reloaded. Look again and retry.` : `${what}: ${err.message}`)
}

const indexes = new WeakMap()
function indexOf(b) {
  let x = indexes.get(b)
  if (x) return x
  const all = (b?.lists || []).flatMap(l => l.sections.flatMap(s => s.items))
  const byKey = new Map(), waiters = new Map(), needers = new Map()
  const add = (m, k, i) => (m.get(k) || m.set(k, []).get(k)).push(i)
  for (const i of all) {
    if (i.key) byKey.set(i.key.toUpperCase(), i)
    for (const k of i.needs || []) add(needers, k.toUpperCase(), i)
    if (!i.done) for (const k of i.waitingOn || []) add(waiters, k.toUpperCase(), i)
  }
  x = { all, byKey, waiters, needers }
  if (b) indexes.set(b, x)
  return x
}
const items = b => indexOf(b).all
const sameKey = (a, b) => String(a).toUpperCase() === String(b).toUpperCase()
const titleOf = key => indexOf(board).byKey.get(String(key).toUpperCase())?.title
const waitersOf = key => indexOf(board).waiters.get(String(key).toUpperCase()) || []
const needersOf = key => indexOf(board).needers.get(String(key).toUpperCase()) || []

async function load() {
  const b = await api('/api/board')
  const stamp = i => i.version + (i.claim ? i.claim.by : '')
  if (board) for (const i of items(b)) if (seen.get(i.key) !== stamp(i)) fresh.add(i.key)
  seen = new Map(items(b).map(i => [i.key, stamp(i)]))
  board = b
  if (branchView) shown = await api('/api/branch?name=' + encodeURIComponent(branchView)).catch(() => { branchView = null; return b })
  else shown = b
  if (tab === 'branches') branchesPage = await api('/api/branches').catch(e => ({ error: e.message }))
  paint()
  if (fresh.size) setTimeout(() => { fresh.clear(); document.querySelectorAll('.fresh').forEach(r => r.classList.remove('fresh')) }, 80)
}

function paint() {
  const b = board
  document.title = `${b.repo}${b.branch ? ' · ' + b.branch : ''} · Callboard`
  $('repo').textContent = b.repo
  $('branch').hidden = !b.branch; $('branch').textContent = '⎇ ' + b.branch
  $('file').textContent = b.worktree; $('file').title = b.worktree
  paintNav()
  $('banner').hidden = !branchView && !b.off
  if (!branchView && b.off) $('banner').innerHTML = 'Callboard is switched off in this repo, so you can read the lists here but not change them. Turn it back on with <code>callboard on</code>.'
  if (branchView) $('banner').innerHTML = `Looking at <b>⎇ ${esc(branchView)}</b>, read-only${shown.worktree ? ' (live from its worktree)' : ' (as last committed)'}. <button class="link" data-act="home">Back to ${esc(b.branch || 'this worktree')}</button>`
  const keep = rememberFocus()
  const sx = document.querySelector('.kanban, .tablewrap')?.scrollLeft
  const ro = shown.readOnly
  if (!TABS.some(t => t.id === tab) && !isListTab(tab)) tab = 'backlog'
  if (isListTab(tab)) {
    const l = listNamed(tab), v = currentView(tab)
    const tools = `<div class="vbar hviews" role="toolbar" aria-label="Views">${viewTabs(l, v)}</div>${ro ? '' : viewTools(l, v)}` +
      (v?.layout === 'board' ? `<button class="vt railbtn" data-act="rail" aria-expanded="${railOpen}">${agentsLine(true)}</button>` : '')
    const own = !ro && !v && tab !== 'backlog'
    const sub = own && listEdit === 'rename' ? `<input id="renamelist" class="listname" value="${esc(tab)}" aria-label="New name for ${esc(l.file)}" autocomplete="off"> <span class="lbl">Enter renames it, Esc keeps it</span>`
      : own && listEdit === 'remove' ? `Remove ${esc(l.file)}${l.open ? ` and its ${l.open} open item${l.open === 1 ? '' : 's'}` : ''}? <button class="link danger" data-act="removelist">Remove it</button> · <button class="link" data-act="keeplist">Keep it</button>`
      : summary(l) + (v ? ` · <span class="lbl">a view of ${tab === 'backlog' ? 'the backlog' : esc(l.file)}</span>` : '') + (own ? ` · <button class="link" data-act="renamelist">Rename</button> · <button class="link" data-act="askremovelist">Remove</button>` : '')
    $('head').innerHTML = head(v ? v.name || 'Untitled' : listTitle(tab), sub, searchBox() + tools)
    $('pane').innerHTML = suggestStrip(tab, ro) + listBody(l, ro)
  } else if (tab === 'foryou') {
    $('head').innerHTML = head('For you', forYouLine(), searchBox())
    $('pane').innerHTML = suggestStrip('questions', ro) + suggestStrip('requests', ro) + forYou(ro)
  } else if (OVERVIEW[tab]) {
    const [title, line, body] = OVERVIEW[tab](ro)
    $('head').innerHTML = head(title, line)
    $('pane').innerHTML = `<div class="ov">${body}</div>`
  } else if (tab === 'map') {
    $('head').innerHTML = head('Map', mapLine(), `<button class="vt" data-act="mapdone" aria-pressed="${mapDone}">Show done</button>`)
    $('pane').innerHTML = paintMap()
  } else {
    $('head').innerHTML = head('Branches', branchesLine())
    $('pane').innerHTML = paintBranches()
  }
  const wide = document.querySelector('.kanban, .tablewrap'); if (wide && sx) wide.scrollLeft = sx
  refreshPop()
  $('app').classList.toggle('wide', !!document.querySelector('.kanban, .map'))
  drawEdges()
  $('app').classList.toggle('railopen', railOpen)
  restoreFocus(keep)
  markSel()
  paintSide()
  paintPeek()
}

const listNamed = kind => searched(shown.lists.find(l => l.kind === kind))
const hay = i => [i.title, i.key, i.section, ...(i.body || []), ...Object.entries(i.fields || {}).flat(), i.answer, i.by, i.claim?.by, claimer(i.claim)].join(' ').toLowerCase()
function searched(l) {
  const ws = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (!l || !ws.length) return l
  return { ...l, sections: l.sections.map(s => ({ ...s, items: s.items.filter(i => ws.every(w => hay(i).includes(w))) })) }
}
const waitingN = l => listItems(l).filter(i => i.waitingOn?.length && !i.done).length
const plural = (n, one, many) => `${n} ${n === 1 ? one : many || one + 's'}`

function head(title, sub, tools = '') {
  return `<div class="htext"><h1>${esc(title)}<button class="info" data-act="hint" aria-expanded="${showHint}" aria-label="What is this?" title="What is this?">?</button></h1>` +
    `<p class="sum">${sub}</p>${showHint ? `<p class="hintline">${HINTS[tab] || (isListTab(tab) ? `Your own list, in <code>${esc(tab)}.md</code>. It works like the backlog: agents claim, tick and add to it when you or they name it, and views work here too.` : '')}</p>` : ''}</div>${tools ? `<div class="tools">${tools}</div>` : ''}`
}
function summary(l) {
  const w = waitingN(l)
  return [`${l.open} open`, `${l.done} done`, w ? `${w} waiting on something` : ''].filter(Boolean).join(' · ')
}
function forYouLine() {
  const q = listNamed('questions')?.open || 0, r = listNamed('requests')?.open || 0, t = yourTasks().length
  if (!q && !r && !t) return 'Nothing waits on you right now.'
  return [q ? plural(q, 'question') : '', r ? plural(r, 'request') : '', t ? plural(t, 'task') : ''].filter(Boolean).join(', ').replace(/, ([^,]*)$/, ' and $1') + ' for you.' + (q || r ? ' Agents wait on these.' : '')
}

function paintNav() {
  const ro = shown.readOnly
  const bl = listNamed('backlog'), q = listNamed('questions'), r = listNamed('requests')
  const fyN = (q?.open || 0) + (r?.open || 0) + yourTasks().length
  const merging = [bl, q, r].some(l => l?.merges?.length)
  const top = (id, label, n, urgent, hide) => `<button role="tab" aria-selected="${tab === id}" data-tab="${id}"${hide ? ' hidden' : ''}>${label}${n ? `<span class="n${urgent ? ' urgent' : ''}">${n}</span>` : ''}</button>`
  const sub = (list, key, name, ic, place) => `<button class="sub${key === 'draft' ? ' draft' : ''}" data-act="view" data-list="${list}" data-v="${esc(key)}" aria-pressed="${tab === place && (currentView(list)?.key || '') === key}" title="${esc(name)}"><i>${ic}</i>${esc(name)}${key === 'draft' ? '<em>draft</em>' : ''}</button>`
  const mine = list => [...views().filter(x => x.list === list), ...(draft?.list === list ? [draft] : [])]
  const icon = x => layoutIcon(x.layout)
  const fyViews = [...mine('questions'), ...mine('requests')]
  $('tabs').innerHTML =
    top('backlog', 'Backlog', bl?.open, !ro && bl?.merges?.length) +
    sub('backlog', '', 'As in the file', '☰', 'backlog') + mine('backlog').map(x => sub('backlog', x.key, x.name || 'Untitled', icon(x), 'backlog')).join('') +
    (ro ? '' : '<button class="sub add" data-act="newview" data-list="backlog"><i>+</i>New view</button>') +
    ownLists().map(l => top(l.kind, esc(listTitle(l.kind)), l.open, !ro && l.merges?.length) +
      (tab === l.kind || mine(l.kind).length ? sub(l.kind, '', 'As in the file', '☰', l.kind) + mine(l.kind).map(x => sub(l.kind, x.key, x.name || 'Untitled', icon(x), l.kind)).join('') +
      (ro ? '' : `<button class="sub add" data-act="newview" data-list="${esc(l.kind)}"><i>+</i>New view</button>`) : '')).join('') +
    (ro ? '' : newListing ? '<div class="sub newlist"><i>+</i><input id="newlist" placeholder="Name, like ideas" aria-label="Name of the new list" autocomplete="off"></div>' : '<button class="sub add" data-act="newlist"><i>+</i>New list</button>') +
    top('foryou', 'For you', fyN, !ro && (fyN > 0 || merging)) +
    (fyViews.length ? [['questions', 'Questions'], ['requests', 'Requests']].flatMap(([list, name]) => mine(list).length ? [sub(list, '', name + ', as in the file', '☰', 'foryou'), ...mine(list).map(x => sub(list, x.key, x.name || 'Untitled', icon(x), 'foryou'))] : []).join('') : '') +
    '<div class="navh">Overview</div>' + overviewNav() +
    top('branches', 'Branches', board.branches.length || '', false, !board.branches.length)
}

function listBody(l, ro, bare) {
  const empty = !l.exists ? `<div class="panel empty">No <code>${esc(l.file)}</code> here yet. ${TASKY(l.kind) ? 'Agents add tasks here as they find them, or add one yourself below.' : l.kind === 'requests' ? 'Agents add a request here when they need you to do something only you can do.' : 'Agents add a question here when a decision is yours to make.'}</div>` : ''
  const isQ = l.kind === 'questions'
  const done = isQ ? listItems(l).filter(i => i.done) : []
  const answeredList = done.length ? `<section><h2>Answered <span>${done.length}</span></h2>${done.slice().sort((a, b) => (b.doneAt || '').localeCompare(a.doneAt || '')).map(q => answered(q, ro)).join('')}</section>` : ''
  const v = currentView(l.kind)
  const top = viewBar(l, v, ro) + empty + merges(l, ro)
  if (v) return `<div data-list="${l.kind}">${top}${renderView(l, v, ro)}</div>`
  const parts = l.sections.filter(s => (isQ ? s.items.filter(i => !i.done) : s.items).length).length
  const place = !ro && !query && !isQ
  return `<div data-list="${l.kind}">` + top + l.sections.filter(s => !query || s.items.length).map(s => {
    const shownItems = isQ ? s.items.filter(i => !i.done) : s.items
    if (!shownItems.length && (!TASKY(l.kind) || ro)) return ''
    const name = s.name || PART[l.kind] || listTitle(l.kind)
    const here = new Set(shownItems.map(i => i.key))
    const rows = withDone(shownItems, l.kind + '/' + s.name, i => i.kind === 'question' ? questionCard(i, ro) : row(i, ro, place, !query && !i.done && here.has(i.parent)))
    const add = TASKY(l.kind) && !ro ? addBox(NOUN[l.kind] || 'task', name, 'section', s.name) : ''
    return `<section${place ? ` class="fsec" data-sec="${esc(s.name)}"` : ''}>${bare && (!s.name || parts === 1) ? '' : `<h2>${esc(name)} <span>${openCount(s.items)}</span>${place && s.name ? `<button class="secbtn" data-act="secmenu" data-sec="${esc(s.name)}" aria-label="Change the heading ${esc(s.name)}" title="Rename, move or remove this heading">⋯</button>` : ''}</h2>`}${rows}${add}</section>`
  }).join('') + answeredList + '</div>'
}

const folds = new Set(JSON.parse(localStorage.getItem('cb-folds') || '[]'))
function withDone(its, fold, render) {
  const done = its.filter(i => i.done)
  if (!done.length || query) return its.map(render).join('')
  const open = folds.has(fold)
  return its.filter(i => !i.done).map(render).join('') +
    `<button class="donefold" data-act="fold" data-fold="${esc(fold)}" aria-expanded="${open}"><span aria-hidden="true">${open ? '▾' : '▸'}</span> ✓ ${done.length} done</button>` +
    (open ? done.map(render).join('') : '')
}
function toggleFold(f) {
  folds.has(f) ? folds.delete(f) : folds.add(f)
  localStorage.setItem('cb-folds', JSON.stringify([...folds]))
  paint()
}
const openCount = its => { const n = its.filter(i => !i.done).length; return n || !its.length ? `${n} open` : 'all done' }

function forYou(ro) {
  const block = (kind, name, none) => {
    const l = listNamed(kind); if (!l) return ''
    const v = currentView(kind)
    const bar = (views().some(x => x.list === kind) ? viewTabs(l, v) : '') + (ro ? '' : viewTools(l, v))
    const count = kind === 'questions' ? `${l.open} waiting for you` : `${l.open} open`
    const nothing = l.exists && !l.open && !v ? `<p class="none pad">${none}</p>` : ''
    return `<div class="fy"><div class="fyhead"><h2>${name}</h2><span class="cnt">${count}</span><div class="vbar" role="toolbar" aria-label="Views of ${esc(l.file)}">${bar}</div></div>${nothing}${listBody(l, ro, true)}</div>`
  }
  const tasks = yourTasks()
  const mineBlock = tasks.length ? `<div class="fy"><div class="fyhead"><h2>Tasks for you</h2><span class="cnt">${tasks.length} open</span></div><div data-list="backlog"><section>${tasks.map(i => row(i, ro)).join('')}</section></div></div>` : ''
  return block('questions', 'Decide', 'No questions for you. Agents ask here when a decision is yours.') +
    block('requests', 'Do', 'Nothing for you to do. Agents ask here for keys, accounts, deploys and the like.') + mineBlock
}
const HUMAN = ['you', 'me', 'human', 'user']
const forNames = w => String(w || '').toLowerCase().replace(/ or | and |[\/&+;]/g, ',').split(',').map(n => n.trim()).filter(Boolean)
const yourTasks = () => ['backlog', ...ownLists().map(l => l.kind)].flatMap(k => listNamed(k)?.sections || []).flatMap(s => s.items).filter(i => !i.done && forNames(i.fields?.for).some(n => HUMAN.includes(n)))

const sideName = l => !l || l === 'ours' || l === 'HEAD' ? 'this branch' : l === 'theirs' ? 'the branch merged in' : /\s/.test(l) ? l : '⎇ ' + l
function merges(l, ro) {
  if (!l.merges?.length) return ''
  return `<section class="merges"><h2>Unfinished merge <span>${l.merges.length}</span></h2>${l.merges.map(m => `<div class="merge" data-key="${esc(m.key)}">` +
    `<div class="mq">Two branches changed this differently. Which version stays?</div><div class="opts">${m.versions.map((v, n) =>
      `<button class="opt" data-act="keep" data-n="${n + 1}"${ro ? ' disabled' : ''}>${v.done ? '✓ ' : ''}${md(v.title)}${v.differs ? `<em>${esc(v.differs)}</em>` : ''}<small>from ${esc(sideName(v.label))}</small></button>`).join('')}</div></div>`).join('')}</section>`
}

async function keep(key, n) {
  try { await send('POST', `/api/items/${encodeURIComponent(key)}/resolve`, { keep: +n }); toast('Kept it. Commit the file to finish the merge.') }
  catch (e) { failed('Could not keep it', e) }
  load(); loadLog()
}

const DUE_FIELDS = ['due', 'deadline', 'due-date', 'due_date', 'duedate', 'by-date'], SOON_DAYS = 3
function parseDate(v) {
  v = String(v || '').trim()
  let m = v.match(/^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})(?:[T ](\d{1,2}):(\d{2}))?/)
  if (m) return new Date(+m[1], m[2] - 1, +m[3], +(m[4] || 0), +(m[5] || 0))
  if (!/[a-z]{3}/i.test(v) || !/\d/.test(v)) return null
  const t = Date.parse(/\b\d{4}\b/.test(v) ? v : v + ' ' + new Date().getFullYear())
  return isNaN(t) ? null : new Date(t)
}
function dueOf(i) {
  for (const f of DUE_FIELDS) { const d = parseDate(i.fields?.[f]); if (d) return d }
  return null
}
function dueWords(i) {
  const d = !i.done && dueOf(i); if (!d) return ''
  const day = x => new Date(x.getFullYear(), x.getMonth(), x.getDate())
  const n = Math.round((day(d) - day(new Date())) / 864e5)
  return n < -1 ? `Overdue by ${-n} days` : n === -1 ? 'Overdue since yesterday' : n === 0 ? 'Due today' : n === 1 ? 'Due tomorrow' : n <= SOON_DAYS ? `Due in ${n} days` : ''
}
const dueBadge = i => { const w = dueWords(i); return w ? `<span class="badge ${w.startsWith('Over') ? 'over' : 'soon'}">${w}</span>` : '' }
function waits(i) {
  if (!i.needs?.length) return ''
  return i.needs.map(k => {
    const open = i.waitingOn?.includes(k), t = titleOf(k)
    const kind = k[0] === 'Q' ? 'your answer' : k[0] === 'R' ? 'you' : 'a task'
    const ans = i.answers?.find(x => sameKey(x.key, k))
    if (ans) return `<span class="badge met">✓ ${md(ans.question)} → <b>${md(ans.answer)}</b></span>`
    return `<span class="badge ${open ? 'wait' : 'met'}">${open ? `Waits on ${kind}` : '✓ Had'}: ${esc(t || k)}</span>`
  }).join('')
}
const agentOf = session => session ? (board.agents || []).find(a => a.session === session) : null
const agentName = a => a.name || a.title || (a.tool === 'codex' ? 'Codex' : 'Claude Code')
const claimer = c => agentOf(c?.session)?.name || c?.by || ''
function avHTML(a, by, small) {
  const codex = a ? a.tool === 'codex' : /codex/i.test(by || '')
  const name = a?.name || by || (a && agentName(a))
  const letter = isYou(by) && !a ? 'Y' : ((name || '?').match(/[\p{L}\p{N}]/u) || ['?'])[0].toUpperCase()
  return `<i class="av${small ? ' sm' : ''}${a && COLORS.includes(a.color) ? ' c-' + a.color : ''}${codex ? ' codex' : ''}${isYou(by) && !a ? ' you' : ''}">${esc(letter)}</i>`
}
const claimHTML = c => {
  if (!c) return ''
  const a = agentOf(c.session)
  const tip = `${a?.title ? `In the session “${a.title}”. ` : ''}Claimed ${ago(c.at)}, last active ${ago(c.seen)}`
  return `<span class="claim" title="${esc(tip)}">${avHTML(a, c.by, true)}${esc(claimer(c))} is on it${c.branch && c.branch !== board.branch ? ` <small>⎇ ${esc(c.branch)}</small>` : ''}</span>`
}
const notes = (i, skipOptions) => {
  const lines = (i.body || []).filter(l => l.trim() && !(skipOptions && optionLine(l)))
  return lines.length ? `<div class="notes">${lines.map(l => `<div>${md(l.trim().replace(/^[-*+]\s+/, '• '))}</div>`).join('')}</div>` : ''
}

const boxHTML = (i, ro, text = ': ' + esc(i.title)) => `<button class="box" role="checkbox" aria-checked="${i.done}" aria-label="${i.done ? 'Open again' : 'Mark done'}${text}"${ro || !i.key ? ' disabled' : ''} data-act="tick"></button>`
const titleHTML = (i, ro, pre = '') => editing === i.key
  ? `<input class="rename" data-key="${esc(i.key)}" value="${esc(i.title)}" aria-label="New title">`
  : `<span class="title"${ro ? '' : ' data-act="rename"'}>${pre}${md(i.title)}</span>`

const subsBadge = i => i.subtasks?.length && !i.done ? `<span class="badge${i.subsOpen ? '' : ' met'}">${i.subtasks.length - (i.subsOpen || 0)} of ${i.subtasks.length} subtasks done</span>` : ''
function row(i, ro, drag, nest) {
  const isReq = i.kind === 'request'
  const title = titleHTML(i, ro, isReq && i.done ? '✅ ' : '')
  const meta = (i.done ? '' : claimHTML(i.claim)) + subsBadge(i) + dueBadge(i) + waits(i)
  return `<div class="row${i.done ? ' done' : ''}${i.blocked ? ' blocked' : ''}${fresh.has(i.key) ? ' fresh' : ''}${nest && i.depth ? ' sub' : ''}" data-key="${esc(i.key)}"${nest && i.depth ? ` style="--d:${i.depth}"` : ''}${drag && i.key ? ' draggable="true"' : ''}>` +
    boxHTML(i, ro) +
    `<div class="t">${title}${meta ? `<div class="meta">${meta}</div>` : ''}${notes(i)}${isReq && !i.done ? blocking(i.key, 'Doing this') : ''}</div>` +
    `${fieldChips(i, currentView(listOf(i)), ro)}</div>`
}

function questionCard(q, ro) {
  const d = drafts.get(q.key) || { text: '', why: '' }
  const opts = q.options || []
  const buttons = opts.map((o, n) => `<button class="opt${o.recommended ? ' rec' : ''}${q.assumed === o.text ? ' assumed' : ''}" data-act="pick" data-n="${n + 1}"${ro ? ' disabled' : ''}>${md(o.text)}${o.recommended ? '<small>recommended</small>' : ''}${q.assumed === o.text ? '<small>agent went ahead with this</small>' : ''}</button>`).join('')
  const built = q.assumed ? needersOf(q.key).filter(i => i.done) : []
  const assumed = q.assumed ? `<div class="assumed">An agent didn't wait and went ahead with <b>${md(q.assumed)}</b>. Pick it to confirm, or pick another to overturn it${built.length ? `; then ${built.map(i => `<b>${esc(i.title)}</b>`).join(', ')}, done on that basis, ${built.length === 1 ? 'opens' : 'open'} again to redo` : ''}.</div>` : ''
  const own = ro ? '' : `<div class="own"><input data-f="text" placeholder="${opts.length ? 'Or answer in your own words' : 'Your answer'}" value="${esc(d.text)}" aria-label="Your answer"><input data-f="why" placeholder="Why (optional)" value="${esc(d.why)}" aria-label="Why"><button data-act="answer">Answer</button></div>`
  return `<div class="qcard${fresh.has(q.key) ? ' fresh' : ''}" data-key="${esc(q.key)}">` +
    `<div class="qhead"><span class="title"${ro ? '' : ' data-act="rename"'}>${editing === q.key ? `<input class="rename" data-key="${esc(q.key)}" value="${esc(q.title)}" aria-label="New question">` : md(q.title)}</span>${whoHTML(q.by)}<span class="age">${ago(q.at)}</span></div>` +
    `${waits(q)}${notes(q, true)}${assumed}${buttons ? `<div class="opts">${buttons}</div>` : ''}${own}` +
    `${blocking(q.key)}</div>`
}

function blocking(key, doing = 'Answering this') {
  const waiting = waitersOf(key)
  return waiting.length ? `<div class="unblocks">${doing} lets ${waiting.map(i => `<b>${esc(i.title)}</b>`).join(', ')} go ahead.</div>` : ''
}

function answered(q, ro) {
  const users = needersOf(q.key).filter(i => !i.done)
  return `<div class="decision" data-key="${esc(q.key)}"><div class="dq">${md(q.title)}</div>` +
    `<div class="da">→ <b>${md(q.answer || 'answered')}</b>${q.why ? ` <span class="why">because ${md(q.why)}</span>` : ''}</div>` +
    `<div class="dm">Answered ${ago(q.doneAt)}${q.fields?.['answered-by'] && q.fields['answered-by'] !== 'you' ? ' by ' + esc(q.fields['answered-by']) : ''}` +
    `${users.length ? ` · ${users.map(i => esc(i.title)).join(', ')} ${users.length === 1 ? 'uses' : 'use'} it` : ''}` +
    `${ro ? '' : ' <button class="link" data-act="reopen">Ask again</button>'}</div></div>`
}

function branchesLine() {
  const p = branchesPage
  if (!p || p.error) return ''
  const open = p.branches.filter(c => !c.inMain), clash = open.filter(c => c.conflicts.length)
  if (!p.branches.length) return `No branches besides ⎇ ${esc(p.main)}`
  return `${plural(open.length, 'branch', 'branches')} not in ⎇ ${esc(p.main)} yet` +
    (clash.length ? ` · <span class="bad">${plural(clash.length, 'would conflict', 'would conflict')}</span>` : open.length ? ' · none would conflict' : '')
}

function paintBranches() {
  const p = branchesPage
  if (!p) return '<div class="panel empty">Loading…</div>'
  if (p.error) return `<div class="panel empty">${esc(p.error)}</div>`
  const open = p.branches.filter(c => !c.inMain), done = p.branches.filter(c => c.inMain)
  if (!p.branches.length) return `<div class="panel empty">Only ⎇ ${esc(p.main)} so far. Branches show here with where they forked, what reached ⎇ ${esc(p.main)}'s lists since, and whether merging them would conflict.</div>`
  return open.map(c => branchCard(c, p.main)).join('') +
    (done.length ? `<div class="inmain">Already in ⎇ ${esc(p.main)}: ${done.map(c => `<button class="link" data-act="branch" data-name="${esc(c.name)}">⎇ ${esc(c.name)}</button>`).join(' · ')}</div>` : '')
}

function itemText(it) {
  const t = `<b>${md(it.title || it.key)}</b>`
  switch (it.what) {
    case 'added': return 'added ' + (it.done ? '✓ ' : '') + t
    case 'ticked': return 'finished ' + t
    case 'reopened': return 'reopened ' + t
    case 'answered': return 'answered ' + t + (it.detail ? ` → ${md(it.detail)}` : '')
    case 'renamed': return `renamed ${md(it.detail)} to ${t}`
    case 'moved': return `moved ${t} to ${md(it.detail)}`
    case 'noted': return `added a note to ${t}`
    case 'notes': return it.detail ? `changed the notes of ${t}` : `removed the notes of ${t}`
    case 'needs': return `${t} now waits on ${md(it.detail.split(', ').map(k => titleOf(k) || k).join(', '))}`
    case 'set': return `set ${md(it.detail)} on ${t}`
    case 'removed': return 'removed ' + t
  }
  return `${esc(it.what)} ${t}`
}
function itemsLine(its, n = 3) {
  const head = its.slice(0, n).map(itemText).join(' · ')
  return its.length > n ? `${head} <details class="more"><summary>${its.length - n} more</summary>${its.slice(n).map(itemText).join('<br>')}</details>` : head
}

function branchCard(c, main) {
  const fork = c.fork ? `Forked from ⎇ ${esc(main)} ${ago(c.fork.at)}, at “${esc(c.fork.subject)}” <code>${esc(c.fork.short)}</code>` : `No history shared with ⎇ ${esc(main)}`
  const steps = c.since.map(s => `<li><span class="dot${s.merged ? ' m' : ''}" aria-hidden="true"></span><div><span class="what">${s.merged ? `merge ⎇ ${esc(s.merged)}` : `“${esc(s.subject)}”`}</span> <small>${ago(s.at)} · ${esc(s.by)}</small><div class="its">${itemsLine(s.items)}</div></div></li>`).join('')
  const since = c.fork ? `<div class="blk"><h3>Since then on ⎇ ${esc(main)}</h3>${c.since.length ? `<ol class="steps">${steps}</ol>${c.older ? `<p class="dim">and ${plural(c.older, 'older change')}</p>` : ''}` : `<p class="dim">Nothing changed in ⎇ ${esc(main)}'s lists.</p>`}</div>` : ''
  const brings = `<div class="blk"><h3>It changes</h3>${c.brings.length ? `<div class="its">${itemsLine(c.brings, 4)}</div>` : '<p class="dim">Nothing in the lists.</p>'}</div>`
  const side = (v, who) => `<span class="ver"><small>${who}</small>${v.title === '(not there)' ? 'deleted it' : (v.done ? '✓ ' : '') + (v.differs ? esc(v.differs) : md(v.title))}</span>`
  const clashes = c.conflicts.map(m => {
    const [a, b] = m.versions
    const title = a.title !== '(not there)' ? a.title : b.title
    return `<li><b>${md(title)}</b> ${side(a, 'on ⎇ ' + esc(main))}${side(b, 'on ⎇ ' + esc(c.name))}</li>`
  }).join('')
  const merge = `<div class="blk"><h3>Merging into ⎇ ${esc(main)} now</h3>${c.conflicts.length
    ? `<p class="bad">${plural(c.conflicts.length, 'item')} changed both ways: whoever merges picks one version of each.</p><ul class="clashes">${clashes}</ul>`
    : '<p class="ok">✓ No conflicts in the lists.</p>'}</div>`
  return `<section class="bcard${c.conflicts.length ? ' clash' : ''}"><div class="bhead"><span class="bname">⎇ ${esc(c.name)}</span>${c.current ? '<span class="badge claim">this worktree</span>' : ''}${c.worktree && !c.current ? '<span class="badge">has a worktree</span>' : ''}` +
    `${c.current ? '' : `<button class="link" data-act="branch" data-name="${esc(c.name)}">Its lists ›</button>`}</div><p class="fork">${fork}</p>${since}${brings}${merge}</section>`
}

const COLORS = ['red', 'blue', 'green', 'yellow', 'purple', 'orange', 'pink', 'cyan']
const HOW = { compact: 'compacted', resume: 'resumed', clear: 'cleared' }
function paintAgents(as) {
  $('sessions').innerHTML = as.length ? as.map(a => {
    const tool = a.tool === 'codex' ? 'Codex' : 'Claude Code'
    const asked = (a.prompt || '').split('\n')[0].trim()
    const name = agentName(a)
    const sub = a.name && a.title ? a.title : asked ? `“${asked.length > 160 ? asked.slice(0, 159) + '…' : asked}”` : ''
    const toolNote = a.name || a.title ? tool + ' · ' : ''
    const state = a.live ? (a.status === 'busy' ? '<span class="lv busy">working</span>' : '<span class="lv">idle</span>') : a.tool === 'codex' ? '' : '<span class="lv off">ended</span>'
    const when = a.live ? (a.how && HOW[a.how] ? `${HOW[a.how]} · started ${ago(a.started)}` : `started ${ago(a.started)}`) : a.active || a.started ? `last active ${ago(a.active || a.started)}` : ''
    const tip = a.prompt ? `Last asked: ${a.prompt}` : ''
    const on = (board.claims || []).filter(c => c.session && c.session === a.session).map(c => titleOf(c.key) || c.key)
    const gone = !a.live && (a.tool !== 'codex' || Date.now() - Date.parse(a.active || a.started) > 30 * 60e3)
    return `<div class="agent ag${gone ? ' gone' : ''}" title="${esc(tip)}">` +
      avHTML(a, a.tool) +
      `<div class="an"><b>${esc(name)}</b>${sub ? `<small class="sub">${esc(sub)}</small>` : ''}${on.length ? `<small class="on" title="${esc(on.join(', '))}">On ${esc(on.join(', '))}</small>` : ''}` +
      `<small><button class="sid" data-sid="${esc(a.session)}" data-tool="${esc(a.tool || 'claude')}" title="Copy the command that resumes it">${esc(a.session.slice(0, 8))}</button> ${toolNote}${when}${a.subagents ? ` · ${a.subagents} subagent${a.subagents > 1 ? 's' : ''}` : ''}</small></div>` +
      state + `</div>`
  }).join('') : '<div class="none">None yet. A Claude Code or Codex session here shows up when it starts.</div>'
}
document.addEventListener('click', e => {
  const b = e.target.closest('.sid')
  if (!b) return
  const cmd = b.dataset.tool === 'codex' ? `codex resume ${b.dataset.sid}` : `claude --resume ${b.dataset.sid}`
  navigator.clipboard?.writeText(cmd).then(() => { const t = b.textContent; b.textContent = 'copied'; setTimeout(() => (b.textContent = t), 1200) })
})
setInterval(() => tab === 'branches' && !document.hidden && load(), 10000)
setInterval(() => board && (!board.readOnly || board.off) && !document.hidden && fetch('/api/agents').then(r => r.json()).then(as => { board.agents = as; paintAgents(as) }).catch(() => {}), 8000)

function paintSide() {
  const b = board
  paintAgents(b.agents || [])
  $('claimpanel').hidden = !b.claims.length
  $('claims').innerHTML = b.claims.map(c => `<div class="agent ag">${avHTML(agentOf(c.session), c.by)}<div class="an"><span class="ct">${esc(titleOf(c.key) || c.key)}</span><small>${esc(claimer(c))}${c.branch && c.branch !== b.branch ? ` on ⎇ ${esc(c.branch)}` : ''} · since ${ago(c.at)}</small></div></div>`).join('')
  $('agentsum').innerHTML = `${agentsLine(false)}<span aria-hidden="true">${railOpen ? '▴' : '▾'}</span>`
  $('agentsum').setAttribute('aria-expanded', railOpen)
}

function agentsLine(short) {
  const as = board.agents || [], busy = as.filter(a => a.live && a.status === 'busy').length
  const here = as.filter(a => a.live || a.tool === 'codex' && Date.now() - Date.parse(a.active || a.started) < 30 * 60e3)
  const stack = here.length ? `<span class="stack">${here.slice(0, 3).map(a => avHTML(a, a.tool, true)).join('')}</span>` : ''
  const who = busy ? `${busy} working` : here.length ? `${here.length} here` : 'Nobody working'
  if (short) return here.length ? `${stack}${who}` : 'Agents and changes'
  const l = board.last
  return `${stack}<span class="txt"><b>${who}</b>${l ? ` · ${esc(l.by)} ${esc(verb(l))} ${esc(l.title)}, ${ago(l.at)}` : ''}</span>`
}

const VERB_IT = { added: 'added it', ticked: 'finished it', reopened: 'opened it again', renamed: 'renamed it', needs: 'changed what it waits on', claimed: 'started on it', released: 'let go of it', assumed: 'went ahead on it', answered: 'answered it', removed: 'deleted it', restored: 'brought it back', resolved: 'settled its merge', moved: 'moved it', noted: 'added a note', notes: 'changed its notes' }
const verb = e => e.what === 'section' ? e.detail + ' in' : e.what === 'list' && e.detail === 'removed' ? 'removed the list' : e.what === 'list' && e.detail?.startsWith('renamed from ') ? `renamed ${e.detail.slice(13)} to` : ({ list: 'made the list', added: 'added', ticked: 'finished', reopened: 'opened again', renamed: 'renamed', needs: 'changed what waits on', claimed: 'started on', released: 'let go of', assumed: 'went ahead on', answered: 'answered', removed: 'deleted', restored: 'brought back', resolved: 'settled the merge of', session: 'started a session', set: 'set', moved: 'moved', noted: 'added a note to', notes: 'changed the notes of', view: 'changed the view', 'view-added': 'made the view', 'view-removed': 'deleted the view', 'view-suggested': 'suggested the view' }[e.what] || e.what)

async function loadLog() {
  const all = await api('/api/log?limit=40'), seen = new Set()
  for (const e of all) if (e.key && !seen.has(e.key)) { seen.add(e.key); e.back = e.what === 'removed' && !!e.copy }
  const ev = all.map(e => e.what === 'view' && e.detail ? { ...e, what: 'view-' + e.detail } : e).filter(e => e.what !== 'session' && e.what !== 'claimed' && e.what !== 'released')
    .filter((e, n, all) => !(e.what === 'view' && all[n - 1]?.what === 'view' && all[n - 1].key === e.key)).slice(0, 10)
  $('events').innerHTML = ev.length ? ev.map(e => evHTML(e)).join('') : '<div class="none">Nothing yet.</div>'
}
const evHTML = (e, noTitle) => `<div class="ev"><span class="dot ${isYou(e.by) ? 'you' : ''}"></span><div><b>${esc(byName(e.by))}</b> ${esc(noTitle && VERB_IT[e.what] || verb(e))}${noTitle ? '' : ' ' + esc(e.title)}${e.what === 'answered' && e.detail ? ` → <i>${esc(e.detail)}</i>` : e.what === 'set' && e.detail ? `: <i>${esc(e.detail)}</i>` : (e.what === 'noted' || e.what === 'notes') && e.detail ? `: <i>${esc(e.detail)}</i>`: e.what === 'moved' && e.detail ? ` to <i>${esc(e.detail)}</i>` : e.what === 'renamed' && e.detail && noTitle ? ` from <i>${esc(e.detail)}</i> to <i>${esc(e.title)}</i>` : ''}<small>${e.via ? esc(e.via) + ' · ' : ''}${e.branch && e.branch !== board?.branch ? '⎇ ' + esc(e.branch) + ' · ' : ''}${ago(e.at)}${e.back && !noTitle && !shown?.readOnly ? ` · <button class="link" data-act="restore" data-k="${esc(e.key)}">Bring back</button>` : ''}</small></div></div>`

function rememberFocus() {
  const a = document.activeElement
  if (!a || a.tagName !== 'INPUT') return null
  if (a.id === 'q') return { sel: '#q', v: a.value, s: a.selectionStart, e: a.selectionEnd }
  if (a.dataset.draftname !== undefined) return { sel: '[data-draftname]', v: a.value, s: a.selectionStart, e: a.selectionEnd }
  const host = a.closest('[data-key]')
  return { key: host?.dataset.key, f: a.dataset.f, g: a.dataset.g, cls: a.className, v: a.value, s: a.selectionStart, e: a.selectionEnd }
}
function restoreFocus(k) {
  if (!k) return
  let el
  if (k.sel === '#q') el = $('q')
  else if (k.sel) el = document.querySelector(k.sel)
  else if (k.g !== undefined) el = [...document.querySelectorAll('.add input')].find(x => x.dataset.g === k.g)
  else if (k.cls === 'rename') el = document.querySelector(`input.rename[data-key="${CSS.escape(k.key)}"]`)
  else if (k.key) el = document.querySelector(`[data-key="${CSS.escape(k.key)}"] input[data-f="${k.f}"]`)
  if (!el) return
  el.value = k.v; el.focus(); try { el.setSelectionRange(k.s, k.e) } catch {}
}

const find = key => items(board).find(i => i.key === key)
async function deleteItem(key) {
  try {
    const r = await send('DELETE', '/api/items/' + encodeURIComponent(key))
    if (peekKey && sameKey(peekKey, key)) closePeek()
    const n = r.freed.length
    toast(`Deleted “${r.title}”${n ? `; ${n === 1 ? 'one item no longer waits' : n + ' items no longer wait'} on it` : ''}.`, () => restoreItem(key))
  } catch (e) { failed('Could not delete it', e) }
  load(); loadLog()
}
async function restoreItem(key) {
  try { await send('POST', '/api/items/' + encodeURIComponent(key) + '/restore'); toast('Brought it back.') }
  catch (e) { failed('Could not bring it back', e) }
  load(); loadLog()
}

async function tick(key) {
  const i = find(key); if (!i) return
  if (!i.done && i.subsOpen) {
    const open = i.subtasks.map(find).filter(x => x && !x.done).map(x => `“${x.title}”`)
    return toast(`Finish its ${open.length === 1 ? 'last subtask' : open.length + ' open subtasks'} first: ${open.join(', ')}.`)
  }
  document.querySelector(`.row[data-key="${CSS.escape(key)}"]`)?.classList.add('busy')
  try { await send('PATCH', '/api/items/' + encodeURIComponent(key), { done: !i.done, version: i.version }) }
  catch (e) { failed('Could not change it', e) }
  load(); loadLog()
}

async function add(g) {
  const input = [...document.querySelectorAll('.add input')].find(x => x.dataset.g === g)
  const title = input.value.trim(); if (!title) return input.focus()
  const gf = input.dataset.gf || 'section', body = { kind: input.closest('[data-list]')?.dataset.list || 'task', title }
  if (gf === 'parent') body.kind = 'task'
  if (gf === 'parent') body.parent = g
  else if (gf === 'section') body.section = g
  else if (g && !BUILTIN.includes(gf)) body.fields = { [gf]: g }
  input.disabled = true
  try { await send('POST', '/api/items', body); input.value = '' }
  catch (e) { failed('Could not add it', e) }
  input.disabled = false; input.focus()
  load(); loadLog()
}

async function addList(name) {
  name = name.trim().toLowerCase().replace(/\s+/g, '-'); if (!name) return
  try { const r = await send('POST', '/api/lists', { name }); newListing = false; await load(); go(r.kind); toast(`Made ${r.file}. Its items get keys starting ${r.prefix}; commit it with .callboard/lists.md and .gitattributes.`) }
  catch (e) { failed('Could not make the list', e) }
}

async function renameList(from, name) {
  name = name.trim().toLowerCase().replace(/\s+/g, '-'); listEdit = null
  if (!name || name === from) return paint()
  try { const r = await send('PATCH', `/api/lists/${encodeURIComponent(from)}`, { name }); await load(); go(r.kind); toast(`Renamed it to ${r.file}; its keys still start ${r.prefix}.`) }
  catch (e) { paint(); failed('Could not rename the list', e) }
}

async function removeList(name) {
  const l = listNamed(name); listEdit = null
  try { await send('DELETE', `/api/lists/${encodeURIComponent(name)}${l?.open ? '?force=1' : ''}`); await load(); go('backlog'); toast(`Removed ${l?.file || name + '.md'}. Once committed, git still has it.`) }
  catch (e) { paint(); failed('Could not remove the list', e) }
}

async function rename(key, title) {
  const i = find(key); editing = null
  if (!i || !title.trim() || title.trim() === i.title) return paint()
  try { await send('PATCH', '/api/items/' + encodeURIComponent(key), { title: title.trim(), version: i.version }) }
  catch (e) { failed('Could not rename it', e) }
  load(); loadLog()
}

async function answer(key, pick, why) {
  const q = find(key); if (!q) return
  const card = document.querySelector(`.qcard[data-key="${CSS.escape(key)}"], .card[data-key="${CSS.escape(key)}"]`)
  card?.classList.add('busy')
  try {
    const r = await send('POST', `/api/items/${encodeURIComponent(key)}/answer`, { answer: pick, why, version: q.version })
    drafts.delete(key)
    const freed = (r.unblocked || []).map(k => titleOf(k) || k)
    const redo = (r.reopened || []).map(k => titleOf(k) || k)
    toast(`Answered “${r.answer}”.${redo.length ? ` An agent had assumed “${r.assumed}”, so ${redo.join(', ')} ${redo.length === 1 ? 'is' : 'are'} open again to redo.` : ''}${freed.length ? ' Now free to go ahead: ' + freed.join(', ') + '.' : ''}${r.branches?.length > 1 ? ` Reached ${r.branches.length} branches.` : ''}`)
  } catch (e) { card?.classList.remove('busy'); failed('Could not answer it', e) }
  load(); loadLog()
}

async function reopen(key) {
  try { await send('POST', `/api/items/${encodeURIComponent(key)}/reopen`, {}); toast('Asked again: it\'s open once more, and what waits on it waits again.') }
  catch (e) { failed('Could not reopen it', e) }
  load(); loadLog()
}

function go(t) {
  listEdit = null
  tab = t; if (isListTab(t)) editList = t
  peekKey = null; paintPeek()
  history.replaceState(null, '', '#' + t)
  if (t === 'branches') load(); else paint()
}

document.addEventListener('click', e => {
  if (!e.target.closest('.pop, [data-act="field"], [data-act="cell"], [data-act="newview"], [data-act="vpop"]')) closePop()
  const t = e.target.closest('[data-tab]'); if (t) return go(t.dataset.tab)
  const a = e.target.closest('[data-act]')
  if (!a || a.dataset.act === 'rename') {
    const r = e.target.closest('#pane [data-key]:not(.merge)')
    if (r && !e.target.closest('input, select, textarea, label, a, .pop')) openPeek(r.dataset.key)
    return
  }
  const key = a.closest('[data-key]')?.dataset.key
  const inList = a.closest('[data-list]')?.dataset.list
  if (inList && ['sortby', 'unfilter', 'vpop'].includes(a.dataset.act)) editList = inList
  switch (a.dataset.act) {
    case 'tick': if (!a.disabled) tick(key); break
    case 'add': add(a.dataset.g); break
    case 'pick': answer(key, a.dataset.n, document.querySelector(`.qcard[data-key="${CSS.escape(key)}"] input[data-f="why"]`)?.value.trim() || ''); break
    case 'answerhere': pickView('questions', ''); setTimeout(() => document.querySelector(`.qcard[data-key="${CSS.escape(key)}"] [data-f="text"]`)?.focus(), 50); break
    case 'answer': {
      const card = a.closest('.qcard'), text = card.querySelector('[data-f="text"]').value.trim()
      if (!text) { card.querySelector('[data-f="text"]').focus(); break }
      answer(key, text, card.querySelector('[data-f="why"]').value.trim()); break
    }
    case 'reopen': reopen(key); break
    case 'view': pickView(a.dataset.list, a.dataset.v); break
    case 'newview': newViewMenu(a); break
    case 'newlist': newListing = true; paintNav(); $('newlist')?.focus(); break
    case 'renamelist': listEdit = 'rename'; paint(); $('renamelist')?.select(); break
    case 'askremovelist': listEdit = 'remove'; paint(); break
    case 'keeplist': listEdit = null; paint(); break
    case 'removelist': removeList(tab); break
    case 'mkdraft': makeDraft(a.dataset.list, a.dataset.r === undefined ? null : pop?.recipes?.[+a.dataset.r], a.dataset.layout); break
    case 'savedraft': saveDraft(); break
    case 'dropdraft': dropDraft(); break
    case 'vpop': vPop(a, a.dataset.pop, a.dataset.n || ''); break
    case 'setlayout': patchView({ layout: a.dataset.v }); break
    case 'setgroup': setGroup(a.dataset.f); break
    case 'gmove': moveGroup(+a.dataset.i, +a.dataset.d); break
    case 'setsort': { const f = a.dataset.f; patchView({ sort: f ? [(DESC_FIRST.has(f) || isPrio(listNamed(editList), f) ? '-' : '') + f] : [] }); break }
    case 'sortdir': { const f = (viewNow()?.sort?.[0] || '').replace(/^-/, ''); if (f) patchView({ sort: [a.dataset.d + f, ...(viewNow().sort || []).slice(1)] }); break }
    case 'filterf': pop.f = a.dataset.f; refreshPop(); break
    case 'filterval': case 'filterop': setFilter(a); break
    case 'propf': toggleProp(a.value); break
    case 'propreset': patchView({ show: [] }); break
    case 'dupview': dupView(); break
    case 'mapdone': mapDone = !mapDone; paint(); break
    case 'maploose': mapLoose = !mapLoose; paint(); break
    case 'rail': railOpen = !railOpen; paint(); break
    case 'delitem': deleteItem(a.dataset.k); break
    case 'restore': restoreItem(a.dataset.k); break
    case 'secmenu': secMenu(a, inList, a.dataset.sec); break
    case 'secmove': if (!a.disabled && pop?.sec) { const t = pop.sibs[pop.n + +a.dataset.d]; if (t) changeSection(+a.dataset.d < 0 ? { before: t.name } : { after: t.name }) } break
    case 'secremove': if (pop?.sec) changeSection({ remove: true, into: pop.el.querySelector('[name="into"]')?.value || '' }); break
    case 'hint': showHint = !showHint; localStorage.setItem('cb-hint', showHint ? '1' : ''); paint(); break
    case 'unfilter': { const v = viewNow(); if (pop?.kind === 'filter') closePop(); patchView({ filter: v.filter.filter((_, n) => n !== +a.dataset.n) }); break }
    case 'delview': delView(a); break
    case 'sortby': sortBy(a.dataset.field); break
    case 'field': case 'cell': if (!shown.readOnly) fieldEditor(a, key, a.dataset.field); break
    case 'clearf': saveField(true); break
    case 'keep': keep(key, a.dataset.n); break
    case 'home': branchView = null; load(); break
    case 'peekclose': closePeek(); break
    case 'peekmove': movePeek(+a.dataset.d); break
    case 'peekto': openPeek(a.dataset.k); break
    case 'fold': toggleFold(a.dataset.fold); break
    case 'copykey': navigator.clipboard?.writeText(a.dataset.k).then(() => toast('Copied ' + a.dataset.k)); break
    case 'trysug': pickView(a.dataset.list, a.dataset.v); break
    case 'keepsug': keepSuggestion(a.dataset.list, a.dataset.v); break
    case 'dropsug': dropSuggestion(a.dataset.v); break
    case 'clearq': query = ''; paint(); break
    case 'branch': branchView = a.dataset.name || null; if (tab === 'branches') tab = 'backlog'; history.replaceState(null, '', '#' + tab); load(); break
  }
})
document.addEventListener('submit', e => {
  if (!e.target.matches('.pop form.rn')) return
  e.preventDefault()
  const v = viewNow(), x = e.target.querySelector('input').value.trim()
  closePop()
  if (v && x && x !== v.name) patchView({ name: uniqueName(v.list, x, v.key) })
})
document.addEventListener('dblclick', e => {
  const t = e.target.closest('[data-act="rename"]'); if (!t) return
  editing = t.closest('[data-key]').dataset.key; paint()
  const inp = document.querySelector('input.rename'); inp?.focus(); inp?.select()
})
document.addEventListener('input', e => {
  if (e.target.id === 'q') { query = e.target.value; return paint() }
  if (e.target.matches('[data-draftname]') && draft) draft.name = e.target.value
  if (e.target.matches('.pop [name="f"]')) fillValues(e.target)
  const f = e.target.dataset.f, card = e.target.closest('.qcard')
  if (f && card) { const d = drafts.get(card.dataset.key) || { text: '', why: '' }; d[f] = e.target.value; drafts.set(card.dataset.key, d) }
})
document.addEventListener('keydown', e => {
  if (e.isComposing) return
  const el = e.target
  if (e.key === 'Escape' && document.querySelector('.pop')) return closePop()
  if (el.matches('.add input') && e.key === 'Enter') add(el.dataset.g)
  else if (el.matches('[data-draftname]') && e.key === 'Enter') saveDraft()
  else if (el.id === 'newlist' && e.key === 'Enter') addList(el.value)
  else if (el.id === 'newlist' && e.key === 'Escape') { newListing = false; paintNav() }
  else if (el.id === 'renamelist' && e.key === 'Enter') { e.preventDefault(); renameList(tab, el.value) }
  else if (el.id === 'renamelist' && e.key === 'Escape') { listEdit = null; paint() }
  else if (el.matches('input.newcol') && e.key === 'Enter') { editList = el.closest('[data-list]')?.dataset.list || editList; addColumn(el) }
  else if (el.matches('input.rename')) {
    if (e.key === 'Enter') rename(el.dataset.key, el.value)
    if (e.key === 'Escape') { editing = null; paint() }
  } else if (el.matches('.qcard input') && e.key === 'Enter') el.closest('.qcard').querySelector('[data-act="answer"]').click()
})
document.addEventListener('focusout', e => { if (e.target.matches('input.rename') && editing) rename(e.target.dataset.key, e.target.value) })

const SKIP = new Set(['answer', 'why', 'answered-by', 'was', 'reopens'])
const BUILTIN = ['section', 'done', 'by', 'at', 'doneat', 'key', 'kind', 'blocked', 'claimed', 'needs']
const LABEL = { section: 'Section', done: 'Done', by: 'Added by', at: 'Added', doneat: 'Done on', key: 'Key', needs: 'Waits on', kind: 'Kind', blocked: 'Waiting', claimed: 'Claimed by', title: 'Title' }
const label = f => LABEL[f] || (f ? f[0].toUpperCase() + f.slice(1).replace(/[-_]/g, ' ') : '')
const RANK = { p4: 0, lowest: 0, low: 1, p3: 1, medium: 2, med: 2, mid: 2, normal: 2, p2: 2, high: 3, p1: 3, urgent: 4, critical: 4, p0: 4 }
const DIRS = { at: ['Oldest first', 'Newest first'], prio: ['Least important first', 'Most important first'], any: ['A → Z, 1 → 9', 'Z → A, 9 → 1'] }
const LAYOUTS = [['list', '☰', 'List'], ['board', '▥', 'Board'], ['table', '▦', 'Table']]
const listOf = i => i.list || ({ task: 'backlog', request: 'requests', question: 'questions' }[i.kind] || 'backlog')
const ownFields = i => Object.keys(i.fields || {}).filter(f => !SKIP.has(f) && i.fields[f] !== '').sort()
const listItems = l => l.sections.flatMap(s => s.items)
const headed = l => l.sections.some(s => s.name)

let dragKey = null
let draft = null

function val(i, f) {
  switch (f) {
    case 'section': return i.section || ''
    case 'done': return i.done ? 'done' : 'open'
    case 'by': return i.by || ''
    case 'at': return (i.at || '').slice(0, 10)
    case 'doneat': return (i.doneAt || '').slice(0, 10)
    case 'kind': return i.kind
    case 'blocked': return i.blocked ? 'waiting' : 'ready'
    case 'claimed': return claimer(i.claim)
    case 'title': return i.title
    case 'key': return i.key
    case 'needs': return (i.waitingOn || []).map(k => titleOf(k) || k).join(', ')
  }
  return i.fields?.[f] || ''
}
function norm(f, v) {
  const x = String(v).toLowerCase()
  if (f === 'done') return ['done', 'yes', 'true', 'x'].includes(x) ? 'done' : ['open', 'no', 'false', ''].includes(x) ? 'open' : x
  if (f === 'blocked') return ['waiting', 'yes', 'true'].includes(x) ? 'waiting' : ['ready', 'no', 'false', ''].includes(x) ? 'ready' : x
  return v
}
function cmpVals(a, b, order, grouping) {
  const oi = x => { const n = (order || []).findIndex(o => o.toLowerCase() === x.toLowerCase()); return n < 0 ? Infinity : n }
  const ia = oi(a), ib = oi(b); if (ia !== ib) return ia < ib ? -1 : 1
  const ra = RANK[a.toLowerCase()], rb = RANK[b.toLowerCase()]
  if (ra !== undefined && rb !== undefined) return grouping ? rb - ra : ra - rb
  if (/^-?\d+(\.\d+)?$/.test(a) && /^-?\d+(\.\d+)?$/.test(b)) return parseFloat(a) - parseFloat(b)
  const da = parseDate(a), db = da && parseDate(b)
  if (da && db) return da - db
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' })
}
function parseFilter(f) {
  const not = f.includes('!='), [k, x = ''] = f.split(not ? '!=' : '='), field = k.trim()
  const words = x.split('|').map(w => String(norm(field, w.trim())))
  return { field, not, words, vals: words.map(w => w.toLowerCase()) }
}
function passes(i, filters) {
  return (filters || []).every(f => {
    const { field, not, vals } = parseFilter(f)
    return vals.includes(String(norm(field, val(i, field))).toLowerCase()) !== not
  })
}
function defaultOrder(l, f) {
  if (f === 'section') return l.sections.map(s => s.name)
  if (f === 'done') return ['open', 'done']
  if (f === 'blocked') return ['ready', 'waiting']
  return []
}
function sorted(its, v, l) {
  const keys = v.sort || []
  if (!keys.length) return its
  return its.slice().sort((x, y) => {
    for (const k of keys) {
      const desc = k.startsWith('-'), f = k.replace(/^-/, '')
      const a = String(norm(f, val(x, f))), b = String(norm(f, val(y, f)))
      if (a === b) continue
      if (!a || !b) return a ? -1 : 1
      const ra = a.toLowerCase() in RANK, rb = b.toLowerCase() in RANK
      if (ra !== rb) return ra ? -1 : 1
      const c = cmpVals(a, b, f === v.group ? [...(v.order || []), ...defaultOrder(l, f)] : defaultOrder(l, f))
      if (c) return desc ? -c : c
    }
    return 0
  })
}
function groups(l, v, its, withEmpty) {
  const f = v.group
  if (!f) return [{ value: null, items: its }]
  const order = [...(v.order || []), ...defaultOrder(l, f)]
  const map = new Map()
  const at = x => { const k = x.toLowerCase(); if (!map.has(k)) map.set(k, { value: x, items: [] }); return map.get(k) }
  if (withEmpty) for (const o of order) if (o) at(o)
  for (const i of its) at(String(norm(f, val(i, f)))).items.push(i)
  return [...map.values()].sort((a, b) => !a.value || !b.value ? (a.value ? -1 : b.value ? 1 : 0) : cmpVals(a.value, b.value, order, true))
}
const settable = (l, f) => f && (!BUILTIN.includes(f) || f === 'section' || (f === 'done' && l.kind !== 'questions'))
const WORDS = { done: { open: 'Open', done: 'Done' }, blocked: { ready: 'Ready', waiting: 'Waiting on something' } }
function groupName(l, f, value) {
  if (f === 'done' && l.kind === 'questions') return value === 'done' ? 'Answered' : 'Waiting for you'
  return WORDS[f]?.[value] || value || (f === 'section' ? PART[l.kind] : 'No ' + label(f).toLowerCase())
}

function valuesFor(l, f) {
  if (f === 'section') return l.sections.map(s => s.name).filter(Boolean)
  if (f === 'done') return ['open', 'done']
  if (f === 'blocked') return ['ready', 'waiting']
  if (BUILTIN.includes(f)) return [...new Set(listItems(l).map(i => val(i, f)).filter(Boolean))].sort()
  const view = views().find(x => x.key === viewKey(l.kind))
  return [...new Set([...(view?.group === f ? view.order || [] : []), ...(l.fields?.[f] || [])])]
}
const fieldsOf = l => Object.keys(l.fields || {}).filter(f => !SKIP.has(f)).sort()
const datalist = (id, vals) => `<datalist id="${id}">${vals.map(x => `<option value="${esc(x)}">`).join('')}</datalist>`

const allViews = () => [...(board?.views || []), ...(draft ? [draft] : [])]
const views = () => allViews().filter(v => !v.suggested && !v.draft)
const suggestions = list => allViews().filter(v => v.suggested && v.list === list)
const viewKey = list => localStorage.getItem('cb-view-' + list) || ''
const currentView = list => allViews().find(v => v.key === viewKey(list) && v.list === list) || null
const viewNow = () => currentView(editList)
function pickView(list, key) {
  localStorage.setItem('cb-view-' + list, key || '')
  editList = list
  go(TASKY(list) ? list : 'foryou')
}

function viewTabs(l, v) {
  const mine = views().filter(x => x.list === l.kind)
  const icon = x => layoutIcon(x.layout)
  const btn = (key, text, title, ic) => `<button class="vt" aria-pressed="${(v?.key || '') === key}" data-act="view" data-list="${l.kind}" data-v="${esc(key)}" title="${esc(title)}"><i>${ic}</i>${esc(text)}</button>`
  return [btn('', 'As in the file', 'The file\'s headings, in file order', '☰')].concat(mine.map(x =>
    btn(x.key, x.name || 'Untitled', `${label(x.layout)}${x.group ? ', grouped by ' + label(x.group).toLowerCase() : ''}${x.by ? ' · made by ' + x.by : ''}`, icon(x)))).join('')
}
function viewTools(l, v) {
  return `<button class="vt add" data-act="newview" data-list="${l.kind}">+ New view</button>`
}

function renderView(l, v, ro) {
  let its = listItems(l).filter(i => passes(i, v.filter))
  its = sorted(its, v, l)
  const total = listItems(l).length, hiddenN = total - its.length
  const note = hiddenN ? `<div class="vnote">${hiddenN} of ${total} hidden by this view's filters.</div>` : ''
  const body = v.layout === 'table' ? tableLayout(l, v, its, ro) : v.layout === 'board' ? boardLayout(l, v, its, ro) : listLayout(l, v, its, ro)
  return datalists(l) + body + note
}
function datalists(l) { return fieldsOf(l).concat(['section']).map(f => datalist('dl-' + f, valuesFor(l, f))).join('') }

function listLayout(l, v, its, ro) {
  const noun = NOUN[l.kind]
  const gs = groups(l, v, its, v.group === 'section')
  if (!its.length && !v.group) return `<div class="panel empty">Nothing matches this view.</div>`
  return gs.map(g => {
    const name = g.value === null ? v.name : groupName(l, v.group, g.value)
    const rows = withDone(g.items, `${v.key}/${g.value ?? ''}`, i => i.kind === 'question' ? (i.done ? answered(i, ro) : questionCard(i, ro)) : row(i, ro))
    const canAdd = TASKY(l.kind) && !ro && (g.value === null || settable(l, v.group) && !BUILTIN.includes(v.group) || v.group === 'section' && g.value)
    const add = canAdd ? addBox(noun, name, v.group || 'section', g.value || '') : ''
    return `<section><h2>${esc(name)} <span>${openCount(g.items)}</span></h2>${rows}${add}</section>`
  }).join('')
}

function boardLayout(l, v, its, ro) {
  const f = v.group || 'section'
  const bv = { ...v, group: f }
  const gs = groups(l, bv, its, true)
  const canDrop = !ro && settable(l, f)
  const cols = gs.map(g => {
    const drop = canDrop && !(f === 'section' && !g.value)
    const add = TASKY(l.kind) && !ro && (f === 'section' ? g.value : !BUILTIN.includes(f)) ? `<div class="add"><input placeholder="+ New" data-g="${esc(g.value)}" data-gf="${esc(f)}" aria-label="New task in ${esc(groupName(l, f, g.value))}"></div>` : ''
    return `<div class="col${drop ? ' drop' : ''}" data-gf="${esc(f)}" data-gv="${esc(g.value)}"><div class="colh"><b>${esc(groupName(l, f, g.value))}</b><span>${g.items.length}</span></div>` +
      `<div class="cards">${g.items.map(i => card(i, bv, ro, canDrop)).join('') || '<div class="none">Nothing here</div>'}</div>${add}</div>`
  }).join('')
  const newCol = canDrop && !BUILTIN.includes(f) ? `<div class="col newc"><input class="newcol" placeholder="+ Column" aria-label="New ${esc(label(f).toLowerCase())} column"></div>` : ''
  const hint = canDrop || ro ? '' : `<div class="vnote">${f === 'done' && l.kind === 'questions' ? 'Answer a question to move it to Answered.' : `These columns come from ${esc(label(f).toLowerCase())}, which Callboard sets itself, so cards can't be dragged between them.`}</div>`
  return `<div class="kanban"${v.sort?.length ? '' : ' data-place="1"'}>${cols}${newCol}</div>${hint}`
}

function card(i, v, ro, drag) {
  const isQ = i.kind === 'question'
  const box = isQ ? '' : boxHTML(i, ro), title = titleHTML(i, ro)
  let q = ''
  if (isQ && i.done) q = `<div class="qa">→ <b>${md(i.answer || 'answered')}</b></div>`
  else if (isQ) q = (i.options || []).length ? `<div class="opts mini">${i.options.map((o, n) => `<button class="opt${o.recommended ? ' rec' : ''}${i.assumed === o.text ? ' assumed' : ''}" data-act="pick" data-n="${n + 1}"${ro ? ' disabled' : ''}>${md(o.text)}</button>`).join('')}</div>` : (ro ? '' : `<button class="link" data-act="answerhere">Answer it</button>`)
  return `<div class="card${i.done ? ' done' : ''}${i.blocked ? ' blocked' : ''}${fresh.has(i.key) ? ' fresh' : ''}" data-key="${esc(i.key)}"${drag && i.key ? ' draggable="true"' : ''}>` +
    `<div class="ch">${box}${title}</div>${fieldChips(i, v, ro, true)}${!i.done && i.claim ? `<div class="meta">${claimHTML(i.claim)}</div>` : ''}${dueBadge(i)}${waits(i)}${q}</div>`
}

function tableLayout(l, v, its, ro) {
  const cols = effShow(l, v)
  const s0 = v.sort?.[0] || ''
  const arrow = f => s0 === f ? ' ↑' : s0 === '-' + f ? ' ↓' : ''
  const head = `<tr><th class="tt" data-act="sortby" data-field="title" aria-sort="${s0.replace(/^-/, '') === 'title' ? (s0[0] === '-' ? 'descending' : 'ascending') : 'none'}">Title${arrow('title')}</th>${cols.map(f => `<th data-act="sortby" data-field="${esc(f)}">${esc(label(f))}${arrow(f)}</th>`).join('')}</tr>`
  const tr = i => {
    const box = i.kind === 'question' ? '' : boxHTML(i, ro), title = titleHTML(i, ro)
    const extra = i.kind === 'question' && i.done ? ` <span class="qa">→ ${md(i.answer)}</span>` : ''
    return `<tr class="${i.done ? 'done' : ''}${fresh.has(i.key) ? ' fresh' : ''}" data-key="${esc(i.key)}"><td class="tt"><div class="tc">${box}<span>${title}${extra}${dueBadge(i)}${waits(i)}</span></div></td>` +
      cols.map(f => { const x = f === 'at' ? ago(i.at) : val(i, f), can = !ro && i.key && settable(l, f); return `<td${can ? ` class="cell" data-act="cell" data-field="${esc(f)}" tabindex="0"` : ''}>${esc(x) || '<span class="unset">—</span>'}</td>` }).join('') + '</tr>'
  }
  const gs = groups(l, v, its, false)
  const body = gs.map(g => (g.value === null ? '' : `<tr class="gh"><td colspan="${cols.length + 1}">${esc(groupName(l, v.group, g.value))} <span>${g.items.length}</span></td></tr>`) + g.items.map(tr).join('')).join('')
  return `<div class="tablewrap"><table class="grid"><thead>${head}</thead><tbody>${body || `<tr><td colspan="${cols.length + 1}" class="none">Nothing matches this view.</td></tr>`}</tbody></table></div>`
}

const addField = i => `<button class="addf" data-act="field" data-field="" aria-label="Set a field on ${esc(i.title)}">+ field</button>`
function fieldChips(i, v, ro, inCard) {
  let fs = v?.show?.length ? v.show.filter(f => f !== 'title') : ownFields(i)
  const grp = v?.layout === 'board' ? v.group : null
  if (grp) fs = [...fs.filter(f => f !== grp), grp]
  const l = shown.lists.find(x => x.kind === listOf(i))
  const chips = fs.map(f => {
    const x = f === 'at' ? ago(i.at) : val(i, f)
    if (!x) return ''
    if (f === 'status' && !v?.show?.length && !inCard && ['todo', 'doing', 'done'].includes(x)) return ''
    const can = !ro && i.key && l && settable(l, f)
    const bars = f === 'prio' ? prioBars(x) : ''
    const lab = inCard || !bars && !QUIET.has(f) ? `<small>${esc(label(f))}</small>` : ''
    return `<span class="fc${can ? '' : ' ro'}${f === grp ? ' grp' : ''}"${can ? ` data-act="field" data-field="${esc(f)}" tabindex="0" role="button"` : ''} title="${esc(label(f))}: ${esc(x)}">${bars}${lab}${esc(x)}</span>`
  }).join('')
  const add = ro || !i.key ? '' : addField(i)
  return chips || add ? `<span class="fcs">${chips}${add}</span>` : ''
}

const QUIET = new Set(['area', 'status', 'due', 'at'])
function prioBars(x) {
  const n = RANK[x.toLowerCase()]
  if (n === undefined) return ''
  return `<span class="bars${n >= 3 ? ' hot' : ''}" aria-hidden="true">${[1, 2, 3].map(k => `<i${k <= Math.min(n, 3) ? ' class="on"' : ''}></i>`).join('')}</span>`
}

function addBox(noun, name, gf, gv) {
  if (query) return ''
  return `<div class="add"><input placeholder="+ Add a ${noun}" title="Agents usually add ${noun}s themselves; you can too" data-g="${esc(gv)}" data-gf="${esc(gf)}" aria-label="New ${noun} in ${esc(name)}"><button data-act="add" data-g="${esc(gv)}">Add</button></div>`
}

let pop = null
function closePop() { pop?.el.remove(); pop = null }
function place(el, anchor) {
  document.body.appendChild(el)
  const b = anchor.getBoundingClientRect()
  el.style.left = Math.max(8, Math.min(b.left, innerWidth - el.offsetWidth - 12)) + scrollX + 'px'
  el.style.top = b.bottom + 6 + scrollY + 'px'
}
function fieldEditor(anchor, key, field) {
  closePop()
  const i = find(key); if (!i) return
  const l = board.lists.find(x => x.kind === listOf(i))
  const el = document.createElement('div')
  el.className = 'pop'
  const names = fieldsOf(l)
  el.innerHTML = `<form>` +
    (field ? `<div class="ph">${esc(label(field))} of <b>${md(i.title)}</b></div>` : `<div class="ph">Set a field on <b>${md(i.title)}</b></div><input name="f" list="dl-fnames" placeholder="Field, like status or prio" autocomplete="off" aria-label="Field name">${datalist('dl-fnames', names)}`) +
    (field === 'done' ? `<select name="v" aria-label="Done"><option value="open">open</option><option value="done"${i.done ? ' selected' : ''}>done</option></select>`
      : `<input name="v" list="dl-pv" placeholder="${field === 'section' ? 'A heading' : 'Value'}" value="${esc(field ? val(i, field) : '')}" autocomplete="off" aria-label="Value">${datalist('dl-pv', field ? valuesFor(l, field) : [])}`) +
    `<div class="pb"><button type="submit">Save</button>${field && field !== 'section' && field !== 'done' ? '<button type="button" class="link" data-act="clearf">Remove</button>' : ''}<span class="pk">Enter saves · Esc closes</span></div></form>`
  pop = { el, key, field, list: l }
  place(el, anchor)
  el.querySelector('form').addEventListener('submit', e => { e.preventDefault(); saveField(false) })
  const first = el.querySelector('input, select'); first.focus(); first.select?.()
}
function fillValues(input) {
  const f = input.value.trim().toLowerCase()
  const dl = pop?.el.querySelector('#dl-pv'); if (!dl) return
  dl.innerHTML = valuesFor(pop.list, f).map(x => `<option value="${esc(x)}">`).join('')
}
async function saveField(clear) {
  if (!pop) return
  const f = (pop.field || pop.el.querySelector('[name="f"]').value).trim().toLowerCase()
  const v = clear ? '' : pop.el.querySelector('[name="v"]').value.trim()
  if (!f) return pop.el.querySelector('[name="f"]').focus()
  const key = pop.key
  closePop()
  await setField(key, f, v)
}
async function setField(key, f, value, place) {
  const i = find(key); if (!i) return
  let body = {}
  if (String(norm(f, val(i, f))).toLowerCase() !== String(norm(f, value)).toLowerCase()) {
    if (f === 'section') body = value ? { section: value } : {}
    else if (f === 'done') body = { done: norm('done', value) === 'done' }
    else body = { fields: { [f]: value } }
  }
  if (place) Object.assign(body, place)
  if (!Object.keys(body).length) return
  try { await send('PATCH', '/api/items/' + encodeURIComponent(key), { ...body, version: i.version }) }
  catch (e) { failed('Could not change it', e) }
  load(); loadLog()
}

function secMenu(anchor, kind, name) {
  if (pop?.sec === name && pop.kind === kind) return closePop()
  closePop()
  const l = listNamed(kind), s = l?.sections.find(x => x.name === name); if (!s) return
  const sibs = l.sections.filter(x => x.name && x.level === s.level), n = sibs.indexOf(s)
  const into = l.sections.filter(x => x.name && x !== s)
  const count = s.items.length
  const el = document.createElement('div')
  el.className = 'pop'
  el.innerHTML = `<form><div class="ph">The heading <b>${esc(name)}</b></div><input name="to" value="${esc(name)}" aria-label="New name for the heading" autocomplete="off">` +
    `<div class="pb"><button type="submit">Rename</button><span class="pk">Enter saves · Esc closes</span></div></form>` +
    `<div class="pb secrow"><button type="button" class="link" data-act="secmove" data-d="-1"${n > 0 ? '' : ' disabled'}>↑ Move up</button><button type="button" class="link" data-act="secmove" data-d="1"${n >= 0 && n < sibs.length - 1 ? '' : ' disabled'}>↓ Move down</button></div>` +
    (!count ? `<div class="pb secrow"><button type="button" class="link" data-act="secremove">Remove this heading</button></div>`
      : into.length ? `<div class="pb secrow"><button type="button" class="link" data-act="secremove">Remove it and move its ${count === 1 ? 'item' : count + ' items'} to</button><select name="into" aria-label="Heading its items go to">${into.map(x => `<option>${esc(x.name)}</option>`).join('')}</select></div>` : '')
  pop = { el, sec: name, kind, sibs, n }
  place(el, anchor)
  el.querySelector('form').addEventListener('submit', e => {
    e.preventDefault()
    const to = el.querySelector('[name="to"]').value.trim()
    if (to && to !== name) changeSection({ to }); else closePop()
  })
  const first = el.querySelector('input'); first.focus(); first.select()
}
async function changeSection(body) {
  if (!pop?.sec) return
  const { kind, sec } = pop
  closePop()
  try { await send('POST', '/api/sections', { kind: NOUN[kind], name: sec, ...body }) }
  catch (e) { failed('Could not change the heading', e) }
  load(); loadLog()
}

const LAYOUT_WHY = { list: 'Rows under group headings', board: 'A column per value; drag cards across to change it, or up and down to reorder', table: 'A column per field; click a cell to change it' }
const STATUS_ORDER = ['backlog', 'todo', 'to do', 'next', 'ready', 'doing', 'in progress', 'review', 'blocked', 'done']
const DESC_FIRST = new Set(['at', 'doneat'])
const isPrio = (l, f) => !!l && valuesFor(l, f).some(x => x.toLowerCase() in RANK)
const layoutIcon = k => LAYOUTS.find(y => y[0] === k)?.[1] || '☰'
function statusOrder(l) {
  const vs = valuesFor(l, 'status')
  if (!vs.length) return ['todo', 'doing', 'done']
  const at = x => { const n = STATUS_ORDER.indexOf(x.toLowerCase()); return n < 0 ? 50 : n }
  return vs.slice().sort((a, b) => at(a) - at(b))
}
function recipes(l) {
  const fs = fieldsOf(l), its = listItems(l), nouns = NOUN[l.kind] + 's'
  const prio = fs.find(f => isPrio(l, f))
  const top = prio ? valuesInOrder(l, prio)[0] : ''
  const R = []
  if (l.kind !== 'questions') R.push({ name: 'Board by status', why: `A column each for ${statusOrder(l).join(', ')}; drag cards across`, layout: 'board', group: 'status', order: statusOrder(l) })
  if (prio) R.push({ name: 'Most important first', why: `Open ${nouns} in a table, ${top} at the top`, layout: 'table', sort: ['-' + prio], filter: ['done=open'] })
  if (!TASKY(l.kind)) R.push({ name: l.kind === 'questions' ? 'Still to answer' : 'Still to do', why: l.kind === 'questions' ? 'The decisions waiting for you, oldest first' : 'What only you can do, oldest first', layout: 'list', filter: ['done=open'], sort: ['at'] })
  if (TASKY(l.kind)) R.push({ name: 'Ready to start', why: 'Open and not waiting on anything' + (prio ? ', most important first' : ''), layout: 'list', filter: ['done=open', 'blocked=ready'], sort: prio ? ['-' + prio] : [] })
  if (its.some(i => i.needs?.length)) R.push({ name: 'Waiting on something', why: 'What is blocked, and on what', layout: 'table', filter: ['done=open', 'blocked=waiting'], show: ['needs', ...(prio ? [prio] : [])] })
  if (TASKY(l.kind)) R.push({ name: 'Who is on what', why: 'Open tasks by the agent working on them', layout: 'board', group: 'claimed', filter: ['done=open'] })
  for (const f of fs.filter(f => f !== prio && f !== 'status')) {
    const vs = valuesFor(l, f)
    if (vs.length > 1) R.push({ name: 'By ' + label(f).toLowerCase(), why: 'Open ' + nouns + ' grouped: ' + fieldHint(l, f), layout: 'list', group: f, filter: ['done=open'] })
  }
  if (its.some(i => i.done)) R.push({ name: l.kind === 'questions' ? 'Answered lately' : 'Recently done', why: 'Newest first', layout: 'list', filter: ['done=done'], sort: ['-doneat'] })
  const taken = new Map(views().filter(v => v.list === l.kind).map(v => [v.name.toLowerCase(), v.key]))
  return R.map(r => ({ ...r, have: taken.get(r.name.toLowerCase()) || '' }))
}
function newViewMenu(anchor) {
  if (pop?.menu) return closePop()
  closePop()
  const list = anchor.dataset.list, l = listNamed(list)
  const rs = recipes(l)
  const el = document.createElement('div')
  el.className = 'pop menu newview'
  el.innerHTML = `<div class="ph">A new view of the ${NOUN[l.kind]}s</div>` +
    rs.map((r, n) => `<button data-act="mkdraft" data-list="${list}" data-r="${n}"><i>${layoutIcon(r.layout)}</i><span><b>${esc(r.name)}</b><small>${r.have ? 'You have this one; opens it' : esc(r.why)}</small></span></button>`).join('') +
    `<div class="ph sep">Or start from nothing</div><div class="blank">${LAYOUTS.map(([k, ic, t]) => `<button data-act="mkdraft" data-list="${list}" data-layout="${k}" title="${esc(LAYOUT_WHY[k])}"><i>${ic}</i>${t}</button>`).join('')}</div>`
  pop = { el, menu: true, recipes: rs }
  place(el, anchor)
}
function makeDraft(list, r, layout) {
  closePop()
  if (r?.have) return pickView(list, r.have)
  const l = listNamed(list)
  const base = r || { name: 'New ' + label(layout).toLowerCase(), layout, ...(layout === 'board' ? (TASKY(l.kind) ? { group: 'status', order: statusOrder(l) } : { group: 'done' }) : {}) }
  const { why, have, ...settings } = base
  startDraft(list, settings, base.name)
  setTimeout(() => { const i = document.querySelector('[data-draftname]'); i?.focus(); i?.select() }, 30)
}
function startDraft(list, settings, name) {
  const { layout, group, order, sort, filter, show } = settings
  draft = { key: 'draft', draft: true, list, layout, group: group || '', order: order || [], sort: sort || [], filter: filter || [], show: show || [], name: uniqueName(list, name) }
  pickView(list, 'draft')
}
async function saveDraft() {
  if (!draft) return
  const { list, layout, group, order, sort, filter, show } = draft
  const name = uniqueName(list, draft.name.trim() || 'Untitled view')
  try {
    const v = await send('POST', '/api/views', { name, list, layout, group, order, sort, filter, show })
    draft = null
    localStorage.setItem('cb-view-' + list, v.key)
    toast(`Saved “${name}”. It's in the sidebar, for you and every agent.`)
  } catch (e) { return failed('Could not save the view', e) }
  load(); loadLog()
}
function dropDraft() { const list = draft?.list; draft = null; closePop(); if (list) pickView(list, '') }
async function dupView() {
  const v = viewNow(); if (!v) return
  closePop()
  startDraft(v.list, v, v.name + ' copy')
}

function filterWords(f) {
  const { field, not, vals, words } = parseFilter(f)
  if (WORDS[field]) {
    const on = Object.keys(WORDS[field]).filter(w => vals.includes(w) !== not)
    if (on.length === 1) return WORDS[field][on[0]]
  }
  if (field === 'claimed' && vals.length === 1 && !vals[0]) return not ? 'Someone is on it' : 'Nobody is on it'
  return `${label(field)} ${not ? 'is not' : 'is'} ${words.map(v => v || 'not set').join(' or ')}`
}
function dirsFor(l, f) { return DIRS[DESC_FIRST.has(f) ? 'at' : f] || (isPrio(l, f) ? DIRS.prio : DIRS.any) }
function sortWords(l, s) {
  const f = s.replace(/^-/, '')
  return `${label(f)}, ${dirsFor(l, f)[s.startsWith('-') ? 1 : 0].toLowerCase()}`
}
function effShow(l, v) {
  if (v.show?.length) return v.show.filter(f => f !== 'title')
  return v.layout === 'table' ? [...fieldsOf(l), ...(headed(l) ? ['section'] : [])] : fieldsOf(l)
}
const groupable = l => [...fieldsOf(l), ...(headed(l) ? ['section'] : []), 'done', 'blocked', 'claimed', 'by']
const FIELD_HINT = { section: 'The headings in the file', done: 'Open or done', blocked: 'Ready, or waiting on something', claimed: 'Who is working on it', by: 'Who added it', at: 'The day it was added', doneat: 'The day it was done', needs: 'What it waits on', key: 'Its key' }
function valuesInOrder(l, f) {
  if (f === 'status') return statusOrder(l)
  if (isPrio(l, f)) return valuesFor(l, f).slice().sort((a, b) => (RANK[b.toLowerCase()] ?? -1) - (RANK[a.toLowerCase()] ?? -1))
  return valuesFor(l, f)
}
const fieldHint = (l, f) => FIELD_HINT[f] || valuesInOrder(l, f).slice(0, 4).join(', ') + (valuesFor(l, f).length > 4 ? '…' : '')
function filterChoices(l, f) {
  if (WORDS[f]) return Object.entries(WORDS[f]).map(([v, text]) => ({ v, text }))
  const vs = valuesInOrder(l, f).map(v => ({ v, text: v }))
  return f === 'section' ? vs : [...vs, { v: '', text: f === 'claimed' ? 'Nobody' : 'Not set' }]
}

function viewBar(l, v, ro) {
  if (!v || ro) return ''
  const pill = (p, text, on, extra = '') => `<button class="pill${on ? ' on' : ''}" data-act="vpop" data-pop="${p}"${extra} aria-haspopup="true">${text}</button>`
  const board = v.layout === 'board', g = v.group || (board ? 'section' : ''), s0 = v.sort?.[0]
  const chips = (v.filter || []).map((f, n) => `<span class="pill on fpill"><button data-act="vpop" data-pop="filter" data-n="${n}" aria-haspopup="true">${esc(filterWords(f))}</button><button class="x" data-act="unfilter" data-n="${n}" aria-label="Remove the filter ${esc(filterWords(f))}">×</button></span>`).join('')
  const top = v.draft ? `<div class="vdraft"><span class="dl">New view</span><input data-draftname value="${esc(v.name)}" aria-label="Name of the new view" autocomplete="off"><button class="primary" data-act="savedraft">Save view</button><button class="link" data-act="dropdraft">Discard</button><span class="dn">Only you see it until you save it. Saved views are shared with agents and every branch.</span></div>` : ''
  return top + `<div class="vset" role="toolbar" aria-label="How this view shows the list">` +
    pill('layout', `<i>${layoutIcon(v.layout)}</i>${label(v.layout)}`, false) +
    pill('group', board ? `Columns: <b>${esc(label(g))}</b>` : g ? `Group: <b>${esc(label(g))}</b>` : 'Group', !!v.group) +
    pill('sort', s0 ? `Sort: <b>${esc(sortWords(l, s0))}</b>` : 'Sort', !!s0) +
    chips + pill('filter', '+ Filter', false, ' data-n="new"') +
    pill('props', v.layout === 'table' ? 'Columns' : 'Properties', !!v.show?.length) +
    (v.draft ? '' : pill('more', '⋯', false, ' aria-label="Rename, duplicate or delete this view"')) + `</div>`
}

function vPop(anchor, kind, n) {
  if (pop?.view && pop.kind === kind && pop.n === n) return closePop()
  closePop()
  editList = anchor.closest('[data-list]')?.dataset.list || editList
  const el = document.createElement('div')
  el.className = 'pop vpop'
  pop = { el, view: true, kind, n, list: editList, f: '' }
  el.innerHTML = vPopHTML()
  place(el, anchor)
  el.querySelector('input:not([type=checkbox])')?.focus()
}
function refreshPop() {
  if (!pop?.view) return
  if (!viewNow()) return closePop()
  const act = document.activeElement?.closest?.('.pop') ? document.activeElement : null
  const focus = act && (act.dataset.act ? `[data-act="${act.dataset.act}"]${act.value !== undefined && act.type === 'checkbox' ? `[value="${CSS.escape(act.value)}"]` : ''}${act.dataset.f !== undefined ? `[data-f="${CSS.escape(act.dataset.f)}"]` : ''}` : null)
  pop.el.innerHTML = vPopHTML()
  const a = document.querySelector(`#pane [data-list="${pop.list}"] [data-pop="${pop.kind}"]${pop.kind === 'filter' ? `[data-n="${pop.n}"]` : ''}`)
  if (a) place(pop.el, a)
  if (focus) pop.el.querySelector(focus)?.focus()
}
function vPopHTML() {
  const v = viewNow(), l = listNamed(pop.list)
  const opt = (act, attrs, on, text, small = '') => `<button class="opt" data-act="${act}" ${attrs} aria-pressed="${on}"><span class="ck" aria-hidden="true">${on ? '✓' : ''}</span><span>${text}${small ? `<small>${small}</small>` : ''}</span></button>`
  switch (pop.kind) {
    case 'layout':
      return `<div class="ph">Show the list as</div>` + LAYOUTS.map(([k, ic, t]) => opt('setlayout', `data-v="${k}"`, v.layout === k, `<i>${ic}</i> ${t}`, LAYOUT_WHY[k])).join('')
    case 'group': {
      const board = v.layout === 'board', g = v.group || (board ? 'section' : '')
      let h = `<div class="ph">${board ? 'A column for each' : 'Group by'}</div>` + (board ? '' : opt('setgroup', 'data-f=""', !g, 'Nothing', 'One list')) +
        groupable(l).map(f => opt('setgroup', `data-f="${esc(f)}"`, g === f, esc(label(f)), esc(fieldHint(l, f)))).join('')
      const gs = g ? groups(l, { ...v, group: g }, listItems(l), true).filter(x => x.value) : []
      if (gs.length > 1) h += `<div class="ph sep">In this order</div><ol class="gorder">` + gs.map((x, n) => `<li><span>${esc(groupName(l, g, x.value))}</span><button data-act="gmove" data-i="${n}" data-d="-1" aria-label="Move ${esc(groupName(l, g, x.value))} up"${n ? '' : ' disabled'}>↑</button><button data-act="gmove" data-i="${n}" data-d="1" aria-label="Move ${esc(groupName(l, g, x.value))} down"${n < gs.length - 1 ? '' : ' disabled'}>↓</button></li>`).join('') + '</ol>'
      return h
    }
    case 'sort': {
      const s0 = v.sort?.[0] || '', f0 = s0.replace(/^-/, '')
      const fs = ['title', ...fieldsOf(l), 'at', 'doneat', ...(headed(l) ? ['section'] : [])]
      let h = `<div class="ph">Sort by</div>` + opt('setsort', 'data-f=""', !f0, 'File order', 'As the items are in the file') + fs.map(f => opt('setsort', `data-f="${esc(f)}"`, f0 === f, esc(label(f)))).join('')
      if (f0) h += `<div class="ph sep">Order</div><div class="seg">` + dirsFor(l, f0).map((t, n) => `<button data-act="sortdir" data-d="${n ? '-' : ''}" aria-pressed="${!!n === s0.startsWith('-')}">${t}</button>`).join('') + '</div>'
      return h
    }
    case 'filter': {
      const cur = pop.n === 'new' ? null : v.filter?.[+pop.n]
      const { field: f, not, vals } = cur ? parseFilter(cur) : { field: pop.f, not: false, vals: [] }
      if (!f) return `<div class="ph">Show only items where…</div>` + groupable(l).map(x => opt('filterf', `data-f="${esc(x)}"`, false, esc(label(x)), esc(fieldHint(l, x)))).join('')
      const yesNo = !!WORDS[f]
      return `<div class="ph">Show only items where <b>${esc(label(f))}</b></div>` +
        (yesNo ? '' : `<div class="seg sm"><button data-act="filterop" data-not="" aria-pressed="${!not}">is</button><button data-act="filterop" data-not="1" aria-pressed="${not}">is not</button></div>`) +
        `<div class="checks">` + filterChoices(l, f).map(c => `<label><input type="checkbox" data-act="filterval" value="${esc(c.v)}"${vals.includes(c.v.toLowerCase()) ? ' checked' : ''}> ${esc(c.text)}</label>`).join('') + '</div>' +
        (cur ? `<div class="pb"><button class="link danger" data-act="unfilter" data-n="${pop.n}">Remove this filter</button></div>` : '')
    }
    case 'props': {
      const eff = effShow(l, v)
      const all = [...new Set([...fieldsOf(l), ...(headed(l) ? ['section'] : []), 'needs', 'claimed', 'by', 'at', 'doneat', 'key'])]
      return `<div class="ph">${v.layout === 'table' ? 'Columns' : 'Shown on each item'}</div><div class="checks">` +
        all.map(f => `<label><input type="checkbox" data-act="propf" value="${esc(f)}"${eff.includes(f) ? ' checked' : ''}> ${esc(label(f))}</label>`).join('') + '</div>' +
        (v.show?.length ? `<div class="pb"><button class="link" data-act="propreset">Back to the default</button></div>` : '')
    }
    case 'more':
      return `<div class="ph">This view</div><form class="rn"><input data-renameview value="${esc(v.name)}" aria-label="Name of the view" autocomplete="off"><button type="submit">Rename</button></form>` +
        opt('dupview', '', false, 'Duplicate', 'Start a new view from this one') +
        `<button class="opt danger" data-act="delview"><span class="ck" aria-hidden="true"></span><span>Delete for everyone</span></button>` +
        `<div class="pnote">Views are shared with agents and every branch${v.by ? `. Made by ${esc(byName(v.by))}` : ''}.</div>`
  }
  return ''
}
function setGroup(f) {
  const v = viewNow(); if (!v) return
  const l = listNamed(editList)
  patchView({ group: f, order: f === 'status' ? statusOrder(l) : [], ...(autoNamed(v) ? { name: uniqueName(v.list, defaultName(v.layout, f || (v.layout === 'board' ? 'section' : '')), v.key) } : {}) })
}
function moveGroup(i, d) {
  const v = viewNow(), l = listNamed(editList); if (!v) return
  const g = v.group || 'section'
  const vs = groups(l, { ...v, group: g }, listItems(l), true).map(x => x.value).filter(Boolean)
  if (i + d < 0 || i + d >= vs.length) return
  ;[vs[i], vs[i + d]] = [vs[i + d], vs[i]]
  patchView({ order: vs })
}
function setFilter(a) {
  const v = viewNow(); if (!v || !pop) return
  const fl = [...(v.filter || [])]
  const cur = pop.n === 'new' ? null : fl[+pop.n]
  let { field: f, not } = cur ? parseFilter(cur) : { field: pop.f, not: false }
  if (a.dataset.act === 'filterop') not = !!a.dataset.not
  const vals = [...pop.el.querySelectorAll('[data-act="filterval"]:checked')].map(x => x.value)
  const next = vals.length ? `${f}${not ? '!=' : '='}${vals.join('|')}` : null
  if (!cur && !next) return
  if (cur && !next) { fl.splice(+pop.n, 1); closePop() }
  else if (cur) fl[+pop.n] = next
  else { fl.push(next); pop.n = String(fl.length - 1) }
  patchView({ filter: fl })
}
function toggleProp(f) {
  const v = viewNow(), l = listNamed(editList); if (!v) return
  const eff = effShow(l, v)
  const next = eff.includes(f) ? eff.filter(x => x !== f) : [...eff, f]
  if (!next.length) { toast('Leave at least one, or pick Back to the default.'); return refreshPop() }
  patchView({ show: next })
}

const defaultName = (layout, group) => group ? 'By ' + label(group).toLowerCase() : label(layout)
function uniqueName(list, name, except) {
  const taken = new Set(views().filter(v => v.list === list && v.key !== except).map(v => v.name.toLowerCase()))
  let n = 1, out = name
  while (taken.has(out.toLowerCase())) out = `${name} ${++n}`
  return out
}
const autoNamed = v => /^(List|Board|Table|By .+?)( \d+)?$/.test(v.name)
async function patchView(patch) {
  const v = viewNow(); if (!v) return
  Object.assign(v, patch)
  paint()
  if (v.draft) return
  try { await send('PATCH', '/api/views/' + encodeURIComponent(v.key), patch) }
  catch (e) { failed('Could not change the view', e) }
  load()
}
async function delView(btn) {
  const v = viewNow(); if (!v) return
  if (!btn.dataset.sure) { btn.dataset.sure = '1'; btn.textContent = `Delete “${v.name}” for everyone?`; return }
  try { await send('DELETE', '/api/views/' + encodeURIComponent(v.key)); localStorage.removeItem('cb-view-' + editList); closePop(); toast(`Deleted the view “${v.name}”.`) }
  catch (e) { failed('Could not delete the view', e) }
  load(); loadLog()
}
function sortBy(f) {
  const v = viewNow(); if (!v || shown.readOnly) return
  const s0 = v.sort?.[0] || ''
  patchView({ sort: s0 === f ? ['-' + f] : s0 === '-' + f ? [] : [f] })
}
function addColumn(el) {
  const v = viewNow(), x = el.value.trim(); if (!v || !x) return
  const l = listNamed(editList)
  const have = [...(v.order || []), ...valuesFor(l, v.group || 'section')]
  if (have.some(o => o.toLowerCase() === x.toLowerCase())) { el.value = ''; return toast(`There's already a “${x}” column.`) }
  const inUse = groups(l, v, listItems(l), true).map(g => g.value).filter(Boolean)
  patchView({ order: [...new Set([...(v.order?.length ? v.order : inUse), x])] })
}

document.addEventListener('dragstart', e => {
  const c = e.target.closest?.('.card[draggable="true"], .row[draggable="true"]'); if (!c) return
  dragKey = c.dataset.key
  e.dataTransfer.setData('text/plain', dragKey); e.dataTransfer.effectAllowed = 'move'
  setTimeout(() => c.classList.add('dragging'))
})
const clearDrag = () => document.querySelectorAll('.dragging, .over, .dropb, .dropa').forEach(x => x.classList.remove('dragging', 'over', 'dropb', 'dropa'))
const dropZone = t => t.closest?.('.col.drop, section.fsec')
function spot(col, y) {
  const inFile = col.matches('section.fsec')
  if (!inFile && !col.closest('.kanban')?.dataset.place) return null
  const cards = [...col.querySelectorAll(inFile ? ':scope > .row[data-key]' : '.cards .card[data-key]')].filter(c => c.dataset.key !== dragKey)
  const next = cards.find(c => { const r = c.getBoundingClientRect(); return y < r.top + r.height / 2 })
  if (next) return { card: next, before: true }
  return cards.length ? { card: cards.at(-1), before: false } : null
}
document.addEventListener('dragend', () => { dragKey = null; clearDrag() })
document.addEventListener('dragover', e => {
  const col = dropZone(e.target); if (!col || !dragKey) return
  e.preventDefault(); e.dataTransfer.dropEffect = 'move'
  document.querySelectorAll('.over, .dropb, .dropa').forEach(x => x.classList.remove('over', 'dropb', 'dropa'))
  col.classList.add('over')
  const at = spot(col, e.clientY)
  if (at) at.card.classList.add(at.before ? 'dropb' : 'dropa')
})
document.addEventListener('drop', e => {
  const col = dropZone(e.target); if (!col || !dragKey) return
  e.preventDefault()
  const inFile = col.matches('section.fsec')
  const key = dragKey, c = document.querySelector(`${inFile ? '.row' : '.card'}[data-key="${CSS.escape(key)}"]`), at = spot(col, e.clientY)
  if (c) {
    col.querySelector('.cards .none')?.remove()
    if (at) at.card.insertAdjacentElement(at.before ? 'beforebegin' : 'afterend', c)
    else if (!col.contains(c)) (col.querySelector('.cards') || col).appendChild(c)
    c.classList.add('busy')
  }
  dragKey = null
  clearDrag()
  const place = at && (at.before ? { before: at.card.dataset.key } : { after: at.card.dataset.key })
  if (inFile) setField(key, 'section', col.dataset.sec, place)
  else setField(key, col.dataset.gf, col.dataset.gv, place)
})
document.addEventListener('keydown', e => {
  if (e.key === 'Enter' && e.target.matches?.('.fc[data-act], td.cell')) { e.preventDefault(); e.target.click() }
})

function live(state, text) { $('live').className = 'live ' + state; $('live').lastElementChild.textContent = text }
function connect() {
  const es = new EventSource('/api/events')
  es.addEventListener('hello', () => { live('on', 'Live'); load(); loadLog() })
  es.addEventListener('board', () => { if (!editing && !dragKey) load() })
  es.addEventListener('change', () => loadLog())
  es.onerror = () => live('off', 'Server stopped · run callboard serve')
}
addEventListener('hashchange', () => {
  const [h, k] = location.hash.slice(1).split('/'), t = OLDTABS[h] || h
  if (t !== tab && (TABS.some(x => x.id === t) || isListTab(t))) go(t)
  if (k && k !== peekKey) openPeek(k)
})
load().then(() => { loadLog(); if (peekKey) openPeek(peekKey) }).catch(e => live('off', e.message))
connect()
setInterval(() => board && !editing && !document.activeElement?.matches('input') && paint(), 60000)

function openPeek(key) {
  peekKey = sel = key; peekLog = null
  history.replaceState(null, '', '#' + tab + '/' + key)
  paintPeek(); markSel()
  api('/api/log?limit=60&key=' + encodeURIComponent(key)).then(ev => { if (peekKey === key) { peekLog = ev; paintPeek() } }).catch(() => {})
}
function closePeek() {
  peekKey = null; history.replaceState(null, '', '#' + tab); paintPeek()
  document.querySelector(`#pane [data-key="${CSS.escape(sel || '')}"]`)?.focus?.()
}
const onScreen = () => [...new Set([...document.querySelectorAll('#pane .row[data-key], #pane .card[data-key], #pane tr[data-key], #pane .qcard[data-key], #pane .decision[data-key], #pane .mcard[data-key]')].map(r => r.dataset.key).filter(Boolean))]
function movePeek(d) {
  const ks = onScreen(); if (!ks.length) return
  const n = ks.indexOf(sel), next = ks[n < 0 ? 0 : Math.max(0, Math.min(ks.length - 1, n + d))]
  sel = next; markSel()
  document.querySelector(`#pane [data-key="${CSS.escape(next)}"]`)?.scrollIntoView({ block: 'nearest' })
  if (peekKey) openPeek(next)
}
function markSel() {
  document.querySelectorAll('#pane .sel').forEach(r => r.classList.remove('sel'))
  if (sel) document.querySelectorAll(`#pane [data-key="${CSS.escape(sel)}"]:not(.merge)`).forEach(r => r.classList.add('sel'))
}

const KINDS = { task: 'Task', request: 'Request', question: 'Question' }
const when2 = s => s ? new Date(s).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : ''
function paintPeek() {
  const el = $('peek'); if (!el) return
  el.hidden = !peekKey
  $('app').classList.toggle('peeking', !!peekKey)
  if (!peekKey || !board) return
  const i = items(shown).find(x => sameKey(x.key, peekKey))
  const top = `<div class="pk-top"><span class="pk-kind">${i ? KINDS[i.kind] || 'Item' : 'Item'}${branchView ? ` on ⎇ ${esc(branchView)}` : ''}</span>` +
    `<span class="pk-nav"><button data-act="peekmove" data-d="-1" aria-label="Previous item" title="Previous (k)">↑</button><button data-act="peekmove" data-d="1" aria-label="Next item" title="Next (j)">↓</button><button data-act="peekclose" aria-label="Close" title="Close (Esc)">×</button></span></div>`
  if (!i) { el.innerHTML = top + `<p class="dim">${esc(peekKey)} isn't in ${branchView ? 'this branch\'s' : 'these'} lists any more.</p>`; return }
  const ro = shown.readOnly || !i.key
  const l = shown.lists.find(x => x.kind === listOf(i))
  const isQ = i.kind === 'question'
  const pv = (f, text, can) => can ? `<button class="pv" data-act="field" data-field="${esc(f)}">${text}</button>` : `<span class="pv ro">${text}</span>`
  const rows = []
  const prop = (name, html) => html && rows.push(`<tr><th>${name}</th><td>${html}</td></tr>`)
  if (isQ) prop('Status', i.done ? `<span class="ok">Answered</span> → <b>${md(i.answer || '')}</b>${i.why ? ` <span class="dim">because ${md(i.why)}</span>` : ''}` : i.assumed ? `Open · an agent went ahead with <b>${md(i.assumed)}</b>` : 'Open, waiting for your answer')
  else {
    const word = i.fields?.status ? pv('status', esc(i.fields.status), !ro && settable(l, 'status')) : `<span>${i.done ? `Done${i.doneAt ? ' ' + ago(i.doneAt) : ''}` : i.blocked ? 'Waiting' : i.claim ? 'Being worked on' : 'Open'}</span>`
    prop('Status', `<span class="pst">${boxHTML(i, ro, '')}${word}</span>`)
  }
  for (const f of ownFields(i).filter(f => isQ || f !== 'status')) prop(esc(label(f)), pv(f, (f === 'prio' ? prioBars(i.fields[f]) : '') + esc(i.fields[f]), !ro && settable(l, f)))
  if (!ro) rows.push(`<tr><th></th><td><button class="pv add" data-act="field" data-field="">+ Add a property</button></td></tr>`)
  if (headed(l)) prop('Section', pv('section', esc(i.section || 'No heading'), !ro))
  if (i.parent) prop('Subtask of', `<button class="link" data-act="peekto" data-k="${esc(i.parent)}">${md(titleOf(i.parent) || i.parent)}</button>`)
  if (!isQ && (i.subtasks?.length || !ro)) prop('Subtasks', (i.subtasks || []).map(k => { const x = find(k); return `<div class="subl">${x?.done ? '✓' : '○'} <button class="link" data-act="peekto" data-k="${esc(k)}">${md(x?.title || k)}</button></div>` }).join('') + (ro ? '' : addBox('subtask', i.title, 'parent', i.key)))
  prop('Waits on', waits(i))
  const lets = waitersOf(i.key)
  prop(isQ ? 'Answering it frees' : 'Finishing it frees', lets.map(x => `<button class="link" data-act="peekto" data-k="${esc(x.key)}">${md(x.title)}</button>`).join(', '))
  if (i.claim && !i.done) { const a = agentOf(i.claim.session); prop('Worked on by', `${claimHTML(i.claim)}<div class="dim">since ${ago(i.claim.at)}, last active ${ago(i.claim.seen)}${a?.title ? ` · session “${esc(a.title)}”` : ''}</div>`) }
  prop('Added', i.at ? `<span class="pst">${avHTML(agentOf(null), i.by, true)}${esc(byName(i.by))}</span> <span class="dim">${when2(i.at)} (${ago(i.at)})</span>` : '<span class="dim">Typed into the file by hand</span>')
  const doneBy = isQ ? i.fields?.['answered-by'] : i.doneBy
  if (i.done && i.doneAt) prop(isQ ? 'Answered' : 'Done', `<span class="dim">${when2(i.doneAt)} (${ago(i.doneAt)})</span>${doneBy ? ' by ' + esc(doneBy) : ''}`)
  prop('Key', `<code>${esc(i.key)}</code> <button class="link" data-act="copykey" data-k="${esc(i.key)}">Copy</button> <span class="dim">in ${esc(l.file)}</span>`)
  const title = editing === i.key ? `<input class="rename" data-key="${esc(i.key)}" value="${esc(i.title)}" aria-label="New title">` : `<h2 class="pk-title"${ro ? '' : ' data-act="rename"'} title="${ro ? '' : 'Double-click to rename'}">${md(i.title)}</h2>`
  const answerBox = isQ && !i.done ? `<div class="pk-sec"><h3>Answer</h3>${questionCard(i, ro)}</div>` : ''
  const nt = notes(i, isQ)
  const hist = peekLog === null ? '<p class="dim">Loading…</p>' : peekLog.length ? peekLog.filter(e => e.what !== 'session').map(e => evHTML(e, true)).join('') : '<p class="dim">No changes logged yet.</p>'
  el.innerHTML = top + `<div data-key="${esc(i.key)}">${title}<table class="props">${rows.join('')}</table></div>${answerBox}` +
    (nt ? `<div class="pk-sec"><h3>Notes</h3>${nt}</div>` : '') + `<div class="pk-sec"><h3>History</h3><div class="pk-hist">${hist}</div></div>` +
    (ro ? '' : `<div class="pk-sec pk-del"><button class="link" data-act="delitem" data-k="${esc(i.key)}">Delete this ${esc((KINDS[i.kind] || 'item').toLowerCase())}</button></div>`)
}

function describeView(v) {
  const lay = { list: 'a list', board: 'a board', table: 'a table' }[v.layout] || 'a list'
  const parts = [lay]
  if (v.group) parts.push((v.layout === 'board' ? 'with a column per ' : 'grouped by ') + label(v.group).toLowerCase())
  const l = listNamed(v.list)
  if (v.sort?.length) parts.push('sorted by ' + v.sort.map(x => sortWords(l, x).toLowerCase()).join(', then '))
  if (v.filter?.length) parts.push('only ' + v.filter.map(f => filterWords(f).toLowerCase()).join(' and '))
  return parts.join(', ')
}
function suggestStrip(list, ro) {
  if (ro) return ''
  return suggestions(list).map(v => {
    const on = currentView(list)?.key === v.key
    return `<div class="suggest${on ? ' on' : ''}" role="note"><span class="sgi" aria-hidden="true">✦</span><div class="sgt"><b>${esc(v.by || 'An agent')}</b> suggests a view: <b>${esc(v.name || 'Untitled')}</b>, ${esc(describeView(v))}${v.why ? `<div class="why">${esc(v.why)}</div>` : ''}</div>` +
      `<span class="sga">${on ? `<button class="link" data-act="view" data-list="${list}" data-v="">Back</button>` : `<button data-act="trysug" data-list="${list}" data-v="${esc(v.key)}">Preview</button>`}<button class="primary" data-act="keepsug" data-list="${list}" data-v="${esc(v.key)}">Keep</button><button class="link" data-act="dropsug" data-v="${esc(v.key)}">Dismiss</button></span></div>`
  }).join('')
}
async function keepSuggestion(list, key) {
  try { await send('PATCH', '/api/views/' + encodeURIComponent(key), { keep: true }); toast('Kept. It\'s in the sidebar now, for you and every agent.') }
  catch (e) { return failed('Could not keep it', e) }
  await load(); pickView(list, key)
}
async function dropSuggestion(key) {
  const v = allViews().find(x => x.key === key)
  try { await send('DELETE', '/api/views/' + encodeURIComponent(key)); toast('Dismissed.') }
  catch (e) { return failed('Could not dismiss it', e) }
  if (v && currentView(v.list)?.key === key) localStorage.setItem('cb-view-' + v.list, '')
  load(); loadLog()
}

const searchBox = () => `<label class="search"><span aria-hidden="true">⌕</span><input id="q" type="search" placeholder="Search" value="${esc(query)}" aria-label="Search this list" autocomplete="off"><kbd aria-hidden="true">/</kbd>${query ? '<button class="link" data-act="clearq" aria-label="Clear the search">×</button>' : ''}</label>`
document.addEventListener('keydown', e => {
  if (e.isComposing || e.metaKey || e.ctrlKey || e.altKey) return
  const typing = e.target.closest?.('input, textarea, select, [contenteditable]')
  if (e.key === 'Escape') {
    if (document.querySelector('.pop') || editing) return
    if (e.target.id === 'q') { query = ''; e.target.blur(); return paint() }
    if (peekKey && !typing) return closePeek()
    return
  }
  if (typing || document.querySelector('.pop')) return
  if (e.key === '/' && $('q')) { e.preventDefault(); $('q').focus(); $('q').select() }
  else if (e.key === 'j' || (peekKey && e.key === 'ArrowDown')) { e.preventDefault(); movePeek(1) }
  else if (e.key === 'k' || (peekKey && e.key === 'ArrowUp')) { e.preventDefault(); movePeek(-1) }
  else if (e.key === 'Enter' && sel && !e.target.closest?.('button, [role="button"], td.cell')) { e.preventDefault(); openPeek(sel) }
  else if (e.key === 'x' && sel && !shown.readOnly) { const i = find(sel); if (i && i.kind !== 'question') tick(sel) }
})

let mapDone = false, mapLoose = false
function mapGraph() {
  const all = items(shown).filter(i => i.key && (mapDone || !i.done))
  const by = new Map(all.map(i => [i.key.toUpperCase(), i]))
  const edges = []
  for (const i of all) for (const n of i.needs || []) {
    const k = n.toUpperCase()
    if (by.has(k) && k !== i.key.toUpperCase()) edges.push([k, i.key.toUpperCase()])
  }
  const linked = new Set(edges.flat())
  const out = new Map(), into = new Map()
  for (const [a, b] of edges) { (out.get(a) || out.set(a, []).get(a)).push(b); (into.get(b) || into.set(b, []).get(b)).push(a) }
  return { all, by, edges, linked, out, into }
}
function mapLine() {
  const { all, linked } = mapGraph()
  const loose = all.length - linked.size
  if (!linked.size) return all.length ? 'Nothing waits on anything yet.' : 'Nothing is open.'
  return `${plural(linked.size, 'item')} linked` + (loose ? ` · ${loose} with no links` : '')
}
function reach(next, key) {
  const out = new Set(), todo = [key]
  while (todo.length) for (const to of next.get(todo.pop()) || []) if (!out.has(to)) { out.add(to); todo.push(to) }
  return out
}
function paintMap() {
  const { all, by, edges, linked, out, into } = mapGraph()
  const depth = new Map(), busy = new Set()
  const depthOf = k => {
    if (depth.has(k)) return depth.get(k)
    if (busy.has(k)) return 0
    busy.add(k)
    const d = Math.max(-1, ...(into.get(k) || []).map(depthOf)) + 1
    busy.delete(k); depth.set(k, d); return d
  }
  const keys = all.map(i => i.key.toUpperCase()).filter(k => linked.has(k))
  keys.forEach(depthOf)
  const cols = []
  for (const k of keys) (cols[depth.get(k)] ||= []).push(k)
  const freed = new Map(), frees = k => freed.get(k) ?? freed.set(k, reach(out, k).size).get(k)
  const row = new Map()
  cols.forEach((c, n) => {
    if (n === 0) c.sort((a, b) => frees(b) - frees(a))
    else {
      const at = new Map(c.map(k => { const ps = (into.get(k) || []).filter(p => row.has(p)).map(p => row.get(p)); return [k, ps.length ? ps.reduce((a, b) => a + b) / ps.length : 1e9] }))
      c.sort((a, b) => at.get(a) - at.get(b))
    }
    c.forEach((k, i) => row.set(k, i))
  })
  const card = k => {
    const i = by.get(k), f = frees(k)
    const state = i.done ? 'Done' : i.claim ? `${esc(claimer(i.claim))} is on it` : i.waitingOn?.length ? 'Waiting' : i.kind === 'question' ? 'Needs your answer' : i.kind === 'request' ? 'Needs you' : 'Ready'
    const cls = i.done ? 'done' : i.claim ? 'claimed' : i.waitingOn?.length ? 'waiting' : 'ready'
    return `<div class="mcard ${i.kind} ${cls}" data-key="${esc(i.key)}" tabindex="0"><div class="mtop"><span class="key">${esc(i.key)}</span><span class="mstate">${state}</span></div>` +
      `<div class="mtitle">${esc(i.title)}</div>${f && !i.done ? `<div class="mfree">Frees ${plural(f, 'item')}</div>` : ''}</div>`
  }
  const loose = all.filter(i => !linked.has(i.key.toUpperCase()))
  const looseHTML = loose.length ? `<button class="donefold" data-act="maploose">${mapLoose ? '▾' : '▸'} ${plural(loose.length, 'item')} with no links</button>` +
    (mapLoose ? `<div class="mloose">${loose.map(i => card(i.key.toUpperCase())).join('')}</div>` : '') : ''
  if (!keys.length) {
    return `<div class="mempty"><p><b>Nothing waits on anything yet.</b></p><p>When an agent says a task needs a request, a question or another task, the map draws it here, from what to do first on the left to what it frees on the right. An agent links items with <code>callboard needs B-k3f9 R-2bq1</code>, or you can ask one to.</p></div>` + looseHTML
  }
  const heads = cols.map((_, n) => n === 0 ? 'Start here' : n === 1 ? 'Then' : 'After that')
  mapNow = { edges, out, into }
  return `<div class="mapwrap"><div class="map"><svg class="medges" aria-hidden="true"></svg>` +
    cols.map((c, n) => `<div class="mcol"><h2>${heads[n]}</h2>${c.map(card).join('')}</div>`).join('') + `</div></div>` +
    `<p class="mlegend"><span class="mk task">Task</span><span class="mk request">Request</span><span class="mk question">Question</span></p>` + looseHTML
}
let mapNow = null, litKey = null
function drawEdges() {
  const m = document.querySelector('.map'); if (!m || !mapNow) return
  const svg = m.querySelector('.medges'), box = m.getBoundingClientRect()
  const rects = new Map([...m.querySelectorAll('.mcard')].map(c => [c.dataset.key.toUpperCase(), c.getBoundingClientRect()]))
  svg.setAttribute('width', m.scrollWidth); svg.setAttribute('height', m.scrollHeight)
  svg.innerHTML = '<defs><marker id="marr" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0L8 4L0 8z"/></marker></defs>' + mapNow.edges.map(([a, b]) => {
    const ra = rects.get(a), rb = rects.get(b); if (!ra || !rb) return ''
    const x1 = ra.right - box.left, y1 = ra.top + ra.height / 2 - box.top, x2 = rb.left - box.left - 2, y2 = rb.top + rb.height / 2 - box.top
    const dx = Math.max(24, (x2 - x1) / 2)
    return `<path data-a="${esc(a)}" data-b="${esc(b)}" d="M${x1} ${y1}C${x1 + dx} ${y1} ${x2 - dx} ${y2} ${x2} ${y2}" marker-end="url(#marr)"/>`
  }).join('')
  litKey = null
}
addEventListener('resize', drawEdges)
function lightMap(key) {
  const m = document.querySelector('.map'); if (!m || !mapNow) return
  const k = key?.toUpperCase() || null
  if (k === litKey) return
  litKey = k
  m.classList.toggle('lit', !!k)
  m.querySelectorAll('.on').forEach(x => x.classList.remove('on'))
  if (!k) return
  const near = new Set([k, ...reach(mapNow.out, k), ...reach(mapNow.into, k)])
  m.querySelectorAll('.mcard').forEach(c => near.has(c.dataset.key.toUpperCase()) && c.classList.add('on'))
  m.querySelectorAll('.medges path').forEach(p => near.has(p.dataset.a) && near.has(p.dataset.b) && p.classList.add('on'))
}
document.addEventListener('mouseover', e => { if (tab === 'map') lightMap(e.target.closest('.map .mcard')?.dataset.key) })
document.addEventListener('focusin', e => { if (tab === 'map' && e.target.matches('.map .mcard')) lightMap(e.target.dataset.key) })

const ov = (id, label, ic, n) => `<button class="sub ovw" data-tab="${id}" aria-pressed="${tab === id}"><i>${ic}</i>${label}${n ? `<span class="n">${n}</span>` : ''}</button>`
function overviewNav() {
  const open = items(board).filter(i => !i.done)
  return ov('waiting', 'Waiting on you', '◎', open.filter(i => i.kind !== 'task').length) +
    ov('now', 'Right now', '◐', open.filter(i => i.claim).length) +
    ov('map', 'Map', '⇢', '') +
    ov('quiet', 'Gone quiet', '◌', quietItems().filter(x => x.quiet).length) +
    ov('recent', 'Recently done', '✓', '')
}
function waitersMap() {
  const next = new Map()
  for (const i of items(shown)) if (!i.done && i.key) for (const k of i.waitingOn || []) {
    const n = k.toUpperCase(); (next.get(n) || next.set(n, []).get(n)).push(i.key.toUpperCase())
  }
  return next
}
const rankOf = i => RANK[String(i.fields?.prio || i.fields?.priority || '').toLowerCase()] ?? 1.5
const DAY = 86400000
const dayOf = s => { const d = new Date(when(s)); return isNaN(d) ? '' : `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}` }
function dayName(day) {
  const [y, m, d] = day.split('-').map(Number), t = new Date(y, m - 1, d), now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const back = Math.round((today - t) / DAY)
  if (back === 0) return 'Today'
  if (back === 1) return 'Yesterday'
  return t.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long', ...(y !== now.getFullYear() ? { year: 'numeric' } : {}) })
}
const since = t => { const d = Math.floor((Date.now() - t) / DAY); return d < 1 ? 'today' : plural(d, 'day') }
function quietItems() {
  const touched = board.touched || {}
  return items(shown).filter(i => !i.done && i.key).map(i => {
    const last = touched[i.key.toUpperCase()]
    const t = Math.max(...[last?.at, i.at, i.doneAt, i.claim?.seen].map(when).filter(x => !isNaN(x)), 0)
    return { i, t, last, quiet: Date.now() - t >= 7 * DAY }
  }).sort((a, b) => a.t - b.t)
}

const OVERVIEW = {
  waiting(ro) {
    const next = waitersMap()
    const mine = items(shown).filter(i => !i.done && i.key && i.kind !== 'task')
      .map(i => ({ i, down: [...reach(next, i.key.toUpperCase())] })).sort((a, b) => b.down.length - a.down.length)
    const line = mine.length ? `${plural(mine.length, 'thing')} only you can do, the one that frees the most first.` : 'Nothing waits on you right now.'
    const body = mine.length ? mine.map(({ i, down }) => {
      const direct = new Set(waitersOf(i.key).map(x => x.key.toUpperCase()))
      const further = down.filter(k => !direct.has(k))
      const card = i.kind === 'question' ? questionCard(i, ro) : `<div class="wyrow">${row(i, ro)}</div>`
      return `<div class="wy"><div class="wyn${down.length ? '' : ' zero'}"><b>${down.length}</b><span>${down.length === 1 ? 'item' : 'items'} freed</span></div><div class="wyb">${card}` +
        (further.length ? `<div class="further">Further down the chain: ${further.map(k => `<a data-act="peekto" data-k="${esc(k)}">${esc(titleOf(k) || k)}</a>`).join(' · ')}</div>` : '') + `</div></div>`
    }).join('') : `<div class="mempty"><p><b>Nothing waits on you.</b></p><p>When an agent needs a decision, a key, an account or a deploy from you, it shows up here, with how much work it holds up.</p></div>`
    return ['Waiting on you', line, body]
  },
  now(ro) {
    const open = items(shown).filter(i => !i.done && i.key)
    const groups = new Map()
    for (const i of open.filter(i => i.claim)) {
      const a = agentOf(i.claim.session), k = claimer(i.claim) + '\0' + (i.claim.branch || '')
      if (!groups.has(k)) groups.set(k, { a, by: i.claim.by, branch: i.claim.branch, seen: i.claim.seen, its: [] })
      const g = groups.get(k); g.its.push(i); if ((i.claim.seen || '') > (g.seen || '')) g.seen = i.claim.seen
    }
    const ready = open.filter(i => i.kind === 'task' && !i.claim && !i.waitingOn?.length).sort((a, b) => rankOf(b) - rankOf(a))
    const later = ready.filter(i => /^later$/i.test(i.section || '')), soon = ready.filter(i => !/^later$/i.test(i.section || ''))
    const assumed = open.filter(i => i.kind === 'question' && i.assumed)
    const working = groups.size ? [...groups.values()].map(g => `<div class="nowg"><div class="nowh">${avHTML(g.a, g.by)}<b>${esc(g.a?.name || g.by)}</b>${g.branch ? `<span class="branch">⎇ ${esc(g.branch)}</span>` : ''}<small>last active ${ago(g.seen)}</small></div>${g.its.map(i => row(i, ro)).join('')}</div>`).join('')
      : '<p class="none pad">Nobody has claimed anything. An agent claims a task when it starts on it.</p>'
    const body = `<section><h2>Being worked on <span>${plural(open.filter(i => i.claim).length, 'item')}</span></h2>${working}</section>` +
      `<section><h2>Ready, nobody on it <span>most important first</span></h2>${soon.length ? soon.map(i => row(i, ro)).join('') : `<p class="none pad">No task is ready: each one is claimed, waiting or done.</p>`}` +
      (later.length ? `<button class="donefold" data-act="fold" data-fold="nowlater">${folds.has('nowlater') ? '▾' : '▸'} ${plural(later.length, 'more task')} under Later</button>${folds.has('nowlater') ? later.map(i => row(i, ro)).join('') : ''}` : '') + `</section>` +
      (assumed.length ? `<section><h2>Answered for you, needs your OK <span>an agent picked an answer so it could go on</span></h2>${assumed.map(q => questionCard(q, ro)).join('')}</section>` : '')
    const line = [plural(groups.size, 'agent') + ' at work', plural(soon.length, 'task') + ' ready', assumed.length ? plural(assumed.length, 'answer') + ' to confirm' : ''].filter(Boolean).join(' · ')
    return ['Right now', line, body]
  },
  quiet(ro) {
    const all = quietItems(), quiet = all.filter(x => x.quiet)
    const one = ({ i, t, last }) => `<div class="qrow">${row(i, ro)}<div class="qlast">${!t ? 'No date on it' : Date.now() - t < DAY ? 'Last touched ' + ago(new Date(t).toISOString()) : `Untouched for ${since(t)}`}${last ? ` · last: ${esc(last.by)} ${esc(VERB_IT[last.what] || verb(last))}` : ''}</div></div>`
    if (!all.length) return ['Gone quiet', 'Nothing is open.', '']
    if (!quiet.length) return ['Gone quiet', 'Nothing has gone quiet: every open item had something happen this past week.',
      `<section><h2>Longest untouched</h2>${all.slice(0, 3).map(one).join('')}</section>`]
    return ['Gone quiet', `${plural(quiet.length, 'open item')} untouched for a week or more.`, quiet.map(one).join('')]
  },
  recent(ro) {
    const done = items(shown).filter(i => i.done)
    const days = new Map(), undated = []
    for (const i of done.slice().sort((a, b) => when(b.doneAt) - when(a.doneAt))) {
      const d = dayOf(i.doneAt); if (!d) { undated.push(i); continue }
      if (!days.has(d)) days.set(d, []); days.get(d).push(i)
    }
    const body = [...days].map(([d, its]) => `<section><h2>${esc(dayName(d))} <span>${plural(its.length, 'item')}</span></h2>${its.map(i => i.kind === 'question' ? answered(i, ro) : row(i, ro)).join('')}</section>`).join('') +
      (undated.length ? `<p class="none pad">${plural(undated.length, 'more item')} done before Callboard kept dates.</p>` : '')
    const week = done.filter(i => Date.now() - when(i.doneAt) < 7 * DAY).length
    return ['Recently done', done.length ? `${week} done this past week, ${done.length} in all.` : 'Nothing done yet.', body || '<div class="mempty"><p>Nothing done yet. Ticked tasks and requests and answered questions show up here, day by day.</p></div>']
  },
}
