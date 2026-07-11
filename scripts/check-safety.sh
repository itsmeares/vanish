#!/usr/bin/env bash
set -euo pipefail

allowed_network_files=(
  "./internal/reddit/oauth.go"
  "./internal/reddit/client.go"
  "./internal/reddit/callback.go"
)

is_allowed_network_file() {
  local file="$1"
  local allowed
  for allowed in "${allowed_network_files[@]}"; do
    if [ "$file" = "$allowed" ]; then
      return 0
    fi
  done
  return 1
}

go_files=()
while IFS= read -r -d '' file; do
  go_files+=("$file")
done < <(find . -name '*.go' ! -name '*_test.go' -print0)

failed=0
network_pattern='net/http|http\.(NewRequest|NewRequestWithContext|Client|Transport)|ListenAndServe|HandleFunc'
for file in "${go_files[@]}"; do
  matches="$(grep -n -E "$network_pattern" "$file" || true)"
  if [ -z "$matches" ]; then
    continue
  fi
  if is_allowed_network_file "$file"; then
    echo "Allowed official Reddit OAuth/API network symbols: $file"
    continue
  fi
  echo "$matches"
  echo "Forbidden network symbol outside official Reddit OAuth/API files: $file"
  failed=1
done

if grep -R -n -E 'http\.(Get|Post|DefaultClient)' --include='*.go' --exclude='*_test.go' .; then
  echo "Forbidden convenience/default HTTP usage found."
  failed=1
fi

if grep -R -n -E 'chromedp|selenium|playwright|agouti|webdriver|webview|puppeteer|colly|goquery|htmlquery|github\.com/go-rod/rod' --include='*.go' --exclude='*_test.go' .; then
  echo "Forbidden browser automation or scraping symbol found."
  failed=1
fi

if grep -R -n -E 'json:"[^"]*(access_token|refresh_token|id_token|client_secret|authorization|auth_header|password|passwd|cookie|session|session_id)[^"]*"' --include='*.go' --exclude='*_test.go' .; then
  echo "Forbidden secret-bearing JSON tag found."
  failed=1
fi

if grep -R -n -E '"/api/(del|editusertext|save|unsave|vote|submit|setpermissions)"' --include='*.go' --exclude='*_test.go' .; then
  echo "Forbidden Reddit content/account mutation endpoint literal found."
  failed=1
fi

if grep -R -n -i -E '(scope|scopes)[^[:cntrl:]]*(edit|save|vote|submit|privatemessages|mod[a-z_]*)' --include='*.go' --exclude='*_test.go' .; then
  echo "Forbidden Reddit OAuth mutation scope declaration/request found."
  failed=1
fi

if [ "$failed" -ne 0 ]; then
  exit 1
fi

echo "Safety checks passed."
