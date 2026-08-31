import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { Account, ActivityFilter, ActivityItem, AmbiguousItem, Job } from '../../shared/types'

const PAGE_SIZE = 100
const emptyFilter: ActivityFilter = { search: '', mediaType: '', from: '', to: '' }
const number = new Intl.NumberFormat()
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' })

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function useRefresh(): [number, () => void] {
  const [value, setValue] = useState(0)
  return [value, () => setValue((current) => current + 1)]
}

export function App(): React.JSX.Element {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [accountId, setAccountId] = useState<string | null>(null)
  const [activeJob, setActiveJob] = useState<Job | null>(null)
  const [error, setError] = useState('')
  const [refresh, bump] = useRefresh()

  useEffect(() => window.vanish.onEvent(() => bump()), [])
  useEffect(() => {
    void window.vanish.accounts.list().then((next) => {
      setAccounts(next)
      setAccountId((current) => current && next.some((account) => account.id === current) ? current : next[0]?.id ?? null)
    }).catch((reason) => setError(message(reason)))
  }, [refresh])
  useEffect(() => {
    if (!activeJob) return
    void window.vanish.jobs.get(activeJob.id).then(setActiveJob).catch((reason) => setError(message(reason)))
  }, [refresh, activeJob?.id])

  const account = accounts.find((candidate) => candidate.id === accountId) ?? null

  async function addAccount(): Promise<void> {
    setError('')
    try {
      const added = await window.vanish.accounts.add()
      setAccountId(added.id)
      bump()
    } catch (reason) { setError(message(reason)) }
  }

  async function signOut(account: Account): Promise<void> {
    setError('')
    try {
      await window.vanish.accounts.signOut(account.id)
      if (activeJob?.accountId === account.id) setActiveJob(null)
      bump()
    } catch (reason) { setError(message(reason)) }
  }

  async function removeAccount(account: Account): Promise<void> {
    if (account.username && !window.confirm(`Remove @${account.username} from Vanish? Its local activity and cleanup history will be deleted.`)) return
    setError('')
    try {
      await window.vanish.accounts.remove(account.id)
      if (activeJob?.accountId === account.id) setActiveJob(null)
      bump()
    } catch (reason) { setError(message(reason)) }
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><span className="brand-mark">V</span><span>Vanish</span></div>
        <nav aria-label="Instagram accounts" className="accounts">
          <p className="nav-label">Instagram</p>
          {accounts.map((item) => (
            <div className="account-entry" key={item.id}>
              <button className={`account-button ${item.id === accountId ? 'active' : ''}`} onClick={() => setAccountId(item.id)}>
                <span className="avatar">{item.username?.slice(0, 1).toUpperCase() ?? '?'}</span>
                <span className="account-copy"><strong>{item.username ? `@${item.username}` : 'Finish sign-in'}</strong><small>{item.scanCount ? `${number.format(item.scanCount)} likes` : item.state.replace('_', ' ')}</small></span>
                <span className={`status-dot ${item.state}`} aria-label={item.state} />
              </button>
              <details className="account-menu">
                <summary aria-label={`Manage ${item.username ? `@${item.username}` : 'unfinished account'}`}>•••</summary>
                <div className="account-menu-popover">
                  {item.username && <button onClick={() => void signOut(item)}>Sign out</button>}
                  <button className="menu-danger" onClick={() => void removeAccount(item)}>{item.username ? 'Remove from Vanish' : 'Cancel setup'}</button>
                </div>
              </details>
            </div>
          ))}
          <button className="add-account" onClick={() => void addAccount()}>Add account</button>
        </nav>
        <p className="local-note">Sessions and activity stay on this device.</p>
      </aside>
      <main id="main" className="main">
        {error && <div className="error-banner" role="alert"><span>{error}</span><button onClick={() => setError('')}>Dismiss</button></div>}
        {!account ? <Welcome onConnect={addAccount} /> : <Library account={account} refresh={refresh} bump={bump} onJob={setActiveJob} onError={setError} />}
      </main>
      {activeJob && <JobPanel job={activeJob} close={() => setActiveJob(null)} onError={setError} />}
    </div>
  )
}

function Welcome({ onConnect }: { onConnect: () => Promise<void> }): React.JSX.Element {
  return <section className="welcome">
    <span className="welcome-mark">V</span>
    <h1>Clean up your Instagram likes.</h1>
    <p>Sign in on Instagram, review what you liked, and remove only what you confirm.</p>
    <button className="primary" onClick={() => void onConnect()}>Connect Instagram</button>
  </section>
}

function Library({ account, refresh, bump, onJob, onError }: {
  account: Account
  refresh: number
  bump: () => void
  onJob: (job: Job) => void
  onError: (value: string) => void
}): React.JSX.Element {
  const [filter, setFilter] = useState(emptyFilter)
  const deferredSearch = useDeferredValue(filter.search)
  const effectiveFilter = useMemo(() => ({ ...filter, search: deferredSearch }), [filter, deferredSearch])
  const [total, setTotal] = useState(0)
  const [pages, setPages] = useState<Record<number, ActivityItem[]>>({})
  const [loadingPages, setLoadingPages] = useState<Set<number>>(new Set())
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [excluded, setExcluded] = useState<Set<number>>(new Set())
  const [allMatching, setAllMatching] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)
  const [latestJob, setLatestJob] = useState<Job | null>(null)
  const [identity, setIdentity] = useState<string | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({ count: total, getScrollElement: () => scrollRef.current, estimateSize: () => 82, overscan: 10 })
  const virtualItems = virtualizer.getVirtualItems()
  const neededPages = [...new Set(virtualItems.map((item) => Math.floor(item.index / PAGE_SIZE)))]
  const neededKey = neededPages.join(',')

  useEffect(() => {
    setIdentity(null)
    setPages({})
    setSelected(new Set())
    setExcluded(new Set())
    setAllMatching(false)
    scrollRef.current?.scrollTo({ top: 0 })
    void loadPage(0, true)
  }, [account.id, JSON.stringify(effectiveFilter)])
  useEffect(() => { for (const page of neededPages) void loadPage(page, false) }, [neededKey])
  useEffect(() => { for (const page of neededPages.length ? neededPages : [0]) void loadPage(page, false, true) }, [refresh])
  useEffect(() => { void window.vanish.jobs.list(account.id).then((jobs) => setLatestJob(jobs[0] ?? null)).catch((reason) => onError(message(reason))) }, [account.id, refresh])

  async function loadPage(page: number, replace: boolean, force = false): Promise<void> {
    if (!force && !replace && (pages[page] || loadingPages.has(page))) return
    setLoadingPages((current) => new Set(current).add(page))
    try {
      const result = await window.vanish.activity.page(account.id, effectiveFilter, page * PAGE_SIZE, PAGE_SIZE)
      setTotal(result.total)
      setPages((current) => replace ? { [page]: result.items } : { ...current, [page]: result.items })
    } catch (reason) { onError(message(reason)) }
    finally { setLoadingPages((current) => { const next = new Set(current); next.delete(page); return next }) }
  }

  function itemAt(index: number): ActivityItem | undefined {
    return pages[Math.floor(index / PAGE_SIZE)]?.[index % PAGE_SIZE]
  }

  function isSelected(id: number): boolean { return allMatching ? !excluded.has(id) : selected.has(id) }
  function toggle(id: number): void {
    if (allMatching) setExcluded((current) => { const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next })
    else setSelected((current) => { const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next })
  }
  const selectedCount = allMatching ? Math.max(0, total - excluded.size) : selected.size

  async function identify(): Promise<void> {
    setBusy(true)
    try { setIdentity(await window.vanish.accounts.identify(account.id)) }
    catch (reason) { onError(message(reason)) }
    finally { setBusy(false) }
  }

  async function bind(): Promise<void> {
    if (!identity) return
    setBusy(true)
    try { await window.vanish.accounts.bind(account.id, identity); bump() }
    catch (reason) { onError(message(reason)) }
    finally { setBusy(false) }
  }

  async function chooseDifferentAccount(): Promise<void> {
    setBusy(true)
    try {
      await window.vanish.accounts.signOut(account.id)
      setIdentity(null)
      await window.vanish.accounts.showLogin(account.id)
      bump()
    } catch (reason) { onError(message(reason)) }
    finally { setBusy(false) }
  }

  async function scan(): Promise<void> {
    try { await window.vanish.accounts.scan(account.id); bump() }
    catch (reason) { onError(message(reason)) }
  }

  async function confirm(): Promise<void> {
    setBusy(true)
    try {
      const job = await window.vanish.jobs.confirm(account.id, {
        filter: effectiveFilter,
        allMatching,
        ids: [...selected],
        excludedIds: [...excluded],
      })
      setConfirming(false)
      onJob(job)
      await window.vanish.jobs.start(job.id)
    } catch (reason) { onError(message(reason)) }
    finally { setBusy(false) }
  }

  const scanning = account.scanState === 'scanning'
  if (account.state !== 'connected') return <section className="connection-screen">
    {identity ? <>
      <div><p className="kicker">Instagram account</p><h1>Use @{identity}?</h1><p>Vanish will keep this browser session and local activity separate from your other accounts.</p></div>
      <div className="button-row"><button className="secondary" disabled={busy} onClick={() => void chooseDifferentAccount()}>Use a different account</button><button className="primary" disabled={busy} onClick={() => void bind()}>{busy ? 'Connecting...' : `Use @${identity}`}</button></div>
    </> : <>
      <div><p className="kicker">Instagram</p><h1>{account.username ? `Reconnect @${account.username}` : 'Finish connecting your account'}</h1><p>{account.message ?? "Use Instagram's own sign-in page. Vanish never sees your password."}</p></div>
      <div className="button-row"><button className="secondary" onClick={() => void window.vanish.accounts.showLogin(account.id)}>Open Instagram</button><button className="primary" disabled={busy} onClick={() => void identify()}>{busy ? 'Checking...' : 'Check signed-in account'}</button></div>
    </>}
  </section>

  return <section className="library">
    <header className="library-header">
      <div><p className="kicker">@{account.username}</p><h1>Liked activity</h1><p className="subline">{account.lastScanAt ? `Last scanned ${date.format(new Date(account.lastScanAt))}` : 'Scan Instagram to build your private library.'}</p></div>
      <div className="header-actions">
        {latestJob && <button className="text-button cleanup-link" onClick={() => onJob(latestJob)}>Cleanup: {latestJob.state.replaceAll('_', ' ')}</button>}
        {scanning ? <button className="secondary" onClick={() => void window.vanish.accounts.pauseScan(account.id)}>Pause scan</button> : <button className="secondary" onClick={() => void scan()}>{account.scanCursor ? 'Resume scan' : account.scanCount ? 'Scan again' : 'Scan likes'}</button>}
      </div>
    </header>
    {account.scanState !== 'idle' && <div className={`notice ${account.scanState}`} role="status"><span><strong>{account.scanState.replace('_', ' ')}</strong>{account.message ? ` · ${account.message}` : scanning ? ` · ${number.format(account.scanCount)} found` : ''}</span>{['needs_auth', 'failed'].includes(account.scanState) && <button onClick={() => void window.vanish.accounts.showLogin(account.id)}>Open Instagram</button>}</div>}
    <div className="filters">
      <label className="search"><span>Search</span><input value={filter.search} onChange={(event) => setFilter((current) => ({ ...current, search: event.target.value }))} placeholder="Creator or caption" /></label>
      <label><span>Type</span><select value={filter.mediaType} onChange={(event) => setFilter((current) => ({ ...current, mediaType: event.target.value as ActivityFilter['mediaType'] }))}><option value="">All</option><option value="post">Posts</option><option value="reel">Reels</option><option value="carousel">Carousels</option></select></label>
      <label><span>From</span><input type="date" value={filter.from} onChange={(event) => setFilter((current) => ({ ...current, from: event.target.value }))} /></label>
      <label><span>To</span><input type="date" value={filter.to} onChange={(event) => setFilter((current) => ({ ...current, to: event.target.value }))} /></label>
    </div>
    <div className="selection-bar">
      <div><strong>{number.format(total)}</strong> liked items</div>
      <div className="selection-actions">
        {selectedCount > 0 && <span>{number.format(selectedCount)} selected</span>}
        <button className="text-button" disabled={!total} onClick={() => { setAllMatching(!allMatching); setSelected(new Set()); setExcluded(new Set()) }}>{allMatching ? 'Clear selection' : 'Select all results'}</button>
        <button className="primary compact" disabled={!selectedCount || scanning} title={scanning ? 'Pause the scan before cleanup.' : undefined} onClick={() => setConfirming(true)}>Unlike selected</button>
      </div>
    </div>
    <div className="list" ref={scrollRef} aria-label="Liked Instagram activity">
      {!total && !scanning ? <div className="empty"><h2>No likes here yet</h2><p>{account.scanCount ? 'Try clearing the filters.' : 'Run a scan to find your liked posts and reels.'}</p></div> :
        <div className="list-spacer" style={{ height: virtualizer.getTotalSize() }}>
          {virtualItems.map((virtual) => {
            const item = itemAt(virtual.index)
            return <div className="virtual-row" key={virtual.key} style={{ height: virtual.size, transform: `translateY(${virtual.start}px)` }}>
              {item ? <ActivityRow item={item} checked={isSelected(item.id)} toggle={() => toggle(item.id)} /> : <div className="row-skeleton" aria-hidden="true" />}
            </div>
          })}
        </div>}
    </div>
    {confirming && <ConfirmDialog count={selectedCount} busy={busy} cancel={() => setConfirming(false)} confirm={confirm} />}
  </section>
}

function ActivityRow({ item, checked, toggle }: { item: ActivityItem; checked: boolean; toggle: () => void }): React.JSX.Element {
  return <label className={`activity-row ${checked ? 'selected' : ''}`}>
    <input className="row-check" type="checkbox" checked={checked} onChange={toggle} aria-label={`Select ${item.shortcode}`} />
    <div className="thumbnail">{item.thumbnailUrl ? <img src={item.thumbnailUrl} alt="" loading="lazy" referrerPolicy="no-referrer" /> : <span>{item.mediaType.slice(0, 1).toUpperCase()}</span>}</div>
    <div className="item-copy"><div><strong>{item.ownerUsername ? `@${item.ownerUsername}` : 'Instagram post'}</strong><span className="type">{item.mediaType}</span></div><p>{item.caption || `Post ${item.shortcode}`}</p></div>
    <time>{date.format(new Date(item.likedAt ?? item.discoveredAt))}</time>
  </label>
}

function ConfirmDialog({ count, busy, cancel, confirm }: { count: number; busy: boolean; cancel: () => void; confirm: () => Promise<void> }): React.JSX.Element {
  useEffect(() => {
    const close = (event: KeyboardEvent): void => { if (event.key === 'Escape' && !busy) cancel() }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [busy, cancel])
  return <div className="dialog-scrim" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) cancel() }}>
    <div className="dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
      <p className="kicker">Final check</p><h2 id="confirm-title">Unlike {number.format(count)} items?</h2>
      <p>Vanish will freeze this exact set. New scan results cannot enter the cleanup.</p>
      <div className="dialog-actions"><button className="secondary" autoFocus disabled={busy} onClick={cancel}>Cancel</button><button className="danger" disabled={busy} onClick={() => void confirm()}>{busy ? 'Freezing selection...' : 'Confirm and start'}</button></div>
    </div>
  </div>
}

function JobPanel({ job, close, onError }: { job: Job; close: () => void; onError: (value: string) => void }): React.JSX.Element {
  const [ambiguous, setAmbiguous] = useState<AmbiguousItem | null>(null)
  const [clock, setClock] = useState(Date.now())
  useEffect(() => { void window.vanish.jobs.ambiguous(job.id).then(setAmbiguous).catch((reason) => onError(message(reason))) }, [job])
  useEffect(() => { const timer = window.setInterval(() => setClock(Date.now()), 1_000); return () => window.clearInterval(timer) }, [])
  const done = job.succeeded + job.alreadyUnliked + job.failed + job.skipped
  const percent = job.total ? Math.round(done / job.total * 100) : 0
  const running = job.state === 'running'
  const canResume = ['paused', 'offline', 'needs_auth', 'needs_reconciliation', 'waiting_rate_limit', 'confirmed'].includes(job.state) && (!job.waitUntil || Date.parse(job.waitUntil) <= clock)

  async function action(run: () => Promise<void>): Promise<void> {
    try { await run() } catch (reason) { onError(message(reason)) }
  }
  async function resolve(resolution: 'done' | 'retry' | 'skip'): Promise<void> {
    if (!ambiguous) return
    await action(() => window.vanish.jobs.resolve(job.id, ambiguous.id, resolution))
  }

  return <aside className="job-panel" aria-label="Cleanup progress">
    <header><div><p className="kicker">Cleanup</p><h2>{job.state === 'completed' ? 'Finished' : job.state.replaceAll('_', ' ')}</h2></div><button className="text-button" onClick={close}>Close</button></header>
    <div className="progress-track"><span style={{ width: `${percent}%` }} /></div>
    <div className="progress-copy"><strong>{number.format(done)} of {number.format(job.total)}</strong><span>{percent}%</span></div>
    {job.message && <div className="job-message" role="status">{job.message}</div>}
    <dl className="summary"><div><dt>Unliked</dt><dd>{number.format(job.succeeded)}</dd></div><div><dt>Already clear</dt><dd>{number.format(job.alreadyUnliked)}</dd></div><div><dt>Failed</dt><dd>{number.format(job.failed)}</dd></div><div><dt>Needs a check</dt><dd>{number.format(job.ambiguous)}</dd></div></dl>
    {ambiguous && job.state === 'needs_reconciliation' && <div className="resolution">
      <h3>Check the last item</h3><p>Instagram did not give Vanish a definite result. Open it, check the heart, then tell Vanish what to do.</p>
      <button className="secondary wide" onClick={() => void action(() => window.vanish.jobs.showItem(job.id, ambiguous.id))}>Open on Instagram</button>
      <button className="primary wide" onClick={() => void resolve('done')}>It is unliked</button>
      <button className="secondary wide" onClick={() => void resolve('retry')}>It is still liked</button>
      <button className="text-button wide" onClick={() => void resolve('skip')}>Skip this item</button>
    </div>}
    {job.state === 'needs_auth' && <button className="secondary wide" onClick={() => void window.vanish.accounts.showLogin(job.accountId)}>Open Instagram</button>}
    <div className="job-actions">
      {running ? <button className="secondary wide" onClick={() => void action(() => window.vanish.jobs.pause(job.id))}>Pause after this item</button> : job.state !== 'completed' && !ambiguous && <button className="primary wide" disabled={!canResume} onClick={() => void action(() => window.vanish.jobs.resume(job.id))}>{job.state === 'waiting_rate_limit' && !canResume ? `Wait until ${new Date(job.waitUntil!).toLocaleTimeString()}` : 'Resume cleanup'}</button>}
    </div>
  </aside>
}
