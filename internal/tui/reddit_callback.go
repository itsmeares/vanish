package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/itsmeares/vanish/internal/reddit"
)

const redditCallbackTimeout = 5 * time.Minute

type redditCallbackWaiter struct {
	listener net.Listener
	path     string
}

type redditCallbackResult struct {
	code string
	err  error
}

func newRedditCallbackWaiter() (*redditCallbackWaiter, error) {
	redirect, err := url.Parse(reddit.DefaultRedirectURI)
	if err != nil {
		return nil, fmt.Errorf("reddit redirect URL is invalid: %w", err)
	}
	if strings.TrimSpace(redirect.Host) == "" || strings.TrimSpace(redirect.Path) == "" {
		return nil, errors.New("reddit redirect URL is missing host or path")
	}

	listener, err := net.Listen("tcp", redirect.Host)
	if err != nil {
		return nil, fmt.Errorf("reddit sign-in listener failed: %w", err)
	}
	return &redditCallbackWaiter{listener: listener, path: redirect.Path}, nil
}

func (waiter *redditCallbackWaiter) wait(ctx context.Context, state string) (string, error) {
	if waiter == nil || waiter.listener == nil {
		return "", errors.New("reddit sign-in listener is not ready")
	}

	ctx, cancel := context.WithTimeout(ctx, redditCallbackTimeout)
	defer cancel()

	resultCh := make(chan redditCallbackResult, 1)
	serveErrCh := make(chan error, 1)
	var once sync.Once
	send := func(result redditCallbackResult) {
		once.Do(func() {
			resultCh <- result
		})
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != waiter.path {
				http.NotFound(writer, request)
				return
			}

			query := request.URL.Query()
			if got := strings.TrimSpace(query.Get("state")); got != strings.TrimSpace(state) {
				http.Error(writer, "Reddit sign-in failed. Return to Vanish.", http.StatusBadRequest)
				send(redditCallbackResult{err: errors.New("reddit OAuth state mismatch; start sign-in again")})
				return
			}
			if problem := strings.TrimSpace(query.Get("error")); problem != "" {
				detail := strings.TrimSpace(query.Get("error_description"))
				if detail == "" {
					detail = problem
				}
				http.Error(writer, "Reddit sign-in failed. Return to Vanish.", http.StatusBadRequest)
				send(redditCallbackResult{err: fmt.Errorf("reddit sign-in failed: %s", detail)})
				return
			}
			code := strings.TrimSpace(query.Get("code"))
			if code == "" {
				http.Error(writer, "Reddit sign-in failed. Return to Vanish.", http.StatusBadRequest)
				send(redditCallbackResult{err: errors.New("reddit auth code is required")})
				return
			}

			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = writer.Write([]byte("Reddit sign-in complete. Return to Vanish."))
			send(redditCallbackResult{code: code})
		}),
	}

	go func() {
		if err := server.Serve(waiter.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()
	defer shutdownRedditCallbackServer(server)

	select {
	case result := <-resultCh:
		return result.code, result.err
	case err := <-serveErrCh:
		return "", fmt.Errorf("reddit sign-in listener failed: %w", err)
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("reddit sign-in timed out")
		}
		return "", errors.New("reddit sign-in cancelled")
	}
}

func shutdownRedditCallbackServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
