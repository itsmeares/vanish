# Instagram real-account smoke test

Use a disposable or low-risk test account with a few deliberately liked posts. Do not commit its app-data directory, screenshots containing personal activity, cookies, or logs.

1. Run `npm run dev`, add the account, and sign in on Instagram's page.
2. Close and reopen Vanish. Confirm the account stays connected without another password prompt.
3. Scan likes. Confirm posts, reels, captions, creators, thumbnails, pagination, and duplicate handling.
4. Search and filter, select two known likes, confirm, and verify no item outside that set enters the cleanup.
5. Run cleanup. Confirm both hearts clear on Instagram and the summary is exact.
6. Pause between items, restart Vanish, and resume.
7. During a test cleanup, disconnect the network after an unlike request. Restore it and confirm Vanish reconciles the current item before any retry.
8. Trigger an Instagram login or verification interruption if the account permits it. Confirm Vanish pauses and resumes only after the normal Instagram page is complete.
9. If Instagram returns a rate limit, confirm Vanish shows a wait state and does not hammer retry.
10. Close Vanish while an item is in flight. Reopen it and confirm the item requires reconciliation or a clear user decision.

Record the Electron version, operating system, account region, scan count, and which steps passed in the draft PR. Instagram compatibility remains unverified until this list is completed with a real account.
