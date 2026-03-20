package deepseek

import "testing"

func TestShouldAttemptRefreshOnTokenInvalidSignal(t *testing.T) {
	if !shouldAttemptRefresh(401, 0, 0, "unauthorized", "") {
		t.Fatal("expected refresh when response indicates invalid token")
	}
}

func TestShouldAttemptRefreshOnBizCodeOnlyFailure(t *testing.T) {
	if !shouldAttemptRefresh(200, 0, 400123, "", "session create failed") {
		t.Fatal("expected refresh on non-zero biz_code with HTTP 200/code=0")
	}
}

func TestShouldAttemptRefreshFalseOnGenericServerError(t *testing.T) {
	if shouldAttemptRefresh(500, 500, 0, "internal error", "") {
		t.Fatal("did not expect refresh on generic server error")
	}
}
