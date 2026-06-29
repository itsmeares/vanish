## Summary

- 
- 
- 

## Type of change

- [ ] Feature
- [ ] Fix
- [ ] Docs
- [ ] Refactor
- [ ] Chore

## Validation

- [ ] `gofmt` on changed Go files
- [ ] `go test ./...`
- [ ] `go build ./cmd/vanish`
- [ ] `git diff --check`
- [ ] Safety grep: `rg -n '\b(net/http|http\.Get|http\.Post|NewRequest|chromedp|selenium|playwright|agouti|rod|webdriver)\b' --glob '*.go' --glob '!*_test.go'`

## Safety / scope check

- [ ] No network/API behavior added unless explicitly intended
- [ ] No login/OAuth/token/cookie/session storage added unless explicitly intended
- [ ] No browser automation, scraping, or private API usage added
- [ ] No apply/delete/execution behavior added unless explicitly intended
- [ ] No raw private messages or sensitive export contents committed
- [ ] No local user exports, ZIPs, plans, or app data committed

## Notes

- 