package reddit

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestCallbackWaiterReceivesCode(t *testing.T) {
	waiter, err := newCallbackWaiter("http://127.0.0.1:0/reddit/oauth/callback")
	if err != nil {
		t.Fatalf("newCallbackWaiter: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan struct {
		code string
		err  error
	}, 1)
	go func() {
		code, err := waiter.Wait(ctx, "state-123")
		result <- struct {
			code string
			err  error
		}{code: code, err: err}
	}()

	res, err := http.Get("http://" + waiter.listener.Addr().String() + waiter.path + "?state=state-123&code=code-123")
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	_ = res.Body.Close()

	select {
	case got := <-result:
		if got.err != nil || got.code != "code-123" {
			t.Fatalf("callback result code=%q err=%v", got.code, got.err)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for callback")
	}
}
