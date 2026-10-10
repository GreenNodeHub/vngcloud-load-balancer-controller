package domain

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/sdk_error"
)

func makeSDKErr(statusCode int, headers http.Header) sdk_error.IError {
	return sdk_error.ErrorHandler(errors.New("upstream rejected")).WithKVparameters(
		"statusCode", statusCode,
		"url", "https://example/api",
		"method", "GET",
		"responseHeaders", headers,
	)
}

func TestSDKError_NotRateLimit(t *testing.T) {
	sdkErr := makeSDKErr(http.StatusInternalServerError, nil)

	err := SDKError(sdkErr)

	assert.False(t, IsRateLimitExceeded(err))
	var rl *RateLimitError
	assert.False(t, errors.As(err, &rl))
}

func TestSDKError_RateLimitWithResetHeader(t *testing.T) {
	headers := http.Header{
		"X-Ratelimit-Reset": []string{"7"},
	}
	sdkErr := makeSDKErr(http.StatusTooManyRequests, headers)

	err := SDKError(sdkErr)

	assert.True(t, IsRateLimitExceeded(err))
	var rl *RateLimitError
	assert.True(t, errors.As(err, &rl))
	assert.Equal(t, 7*time.Second, rl.RetryAfter)
	assert.Equal(t, "GET", rl.Method)
}

func TestSDKError_RateLimitWithRetryAfterFallback(t *testing.T) {
	headers := http.Header{
		"Retry-After": []string{"42"},
	}
	sdkErr := makeSDKErr(http.StatusTooManyRequests, headers)

	rl := &RateLimitError{}
	assert.True(t, errors.As(SDKError(sdkErr), &rl))
	assert.Equal(t, 42*time.Second, rl.RetryAfter)
}

func TestSDKError_RateLimitNoHeader(t *testing.T) {
	sdkErr := makeSDKErr(http.StatusTooManyRequests, http.Header{})

	rl := &RateLimitError{}
	assert.True(t, errors.As(SDKError(sdkErr), &rl))
	assert.Zero(t, rl.RetryAfter)
}

func TestSDKError_Nil(t *testing.T) {
	assert.NoError(t, SDKError(nil))
}

func TestRateLimitRequeueAfter_FloorAndJitter(t *testing.T) {
	for range 50 {
		got := RateLimitRequeueAfter(0)
		assert.GreaterOrEqual(t, got, 2*time.Second, "below floor")
		assert.Less(t, got, 3*time.Second, "above floor + max jitter (50%%)")
	}
}

func TestRateLimitRequeueAfter_RespectsServerHint(t *testing.T) {
	for range 50 {
		got := RateLimitRequeueAfter(10 * time.Second)
		assert.GreaterOrEqual(t, got, 10*time.Second)
		assert.Less(t, got, 15*time.Second)
	}
}

func TestRateLimitRequeueAfter_Ceiling(t *testing.T) {
	got := RateLimitRequeueAfter(1 * time.Hour)
	// 5m ceiling + up to 50% jitter
	assert.GreaterOrEqual(t, got, 5*time.Minute)
	assert.Less(t, got, 8*time.Minute)
}

// sdkRateLimitErr is what the SDK actually hands back on HTTP 429: vngcloud/client/http.go maps
// 429 to WithErrorPermissionDenied, and SdkErrorHandler returns early once an error code is set,
// so no product matcher ever runs and this string is a constant for every 429.
func sdkRateLimitErr() sdk_error.IError {
	return makeSDKErr(http.StatusTooManyRequests, http.Header{"X-Ratelimit-Reset": []string{"32"}}).
		WithErrors(errors.New("permission denied when making request to external service"))
}

// #33380: the message goes on the CR as status.lastReconcileMessage, which is the first thing an
// operator reads - before the logs. Saying "permission denied" there sends them after an IAM
// problem that does not exist, which is how the previous rate-limit incident lost its first hours.
// The suffix carries nothing either: on 429 the SDK's text is always that same sentence.
func TestRateLimitErrorMessageDoesNotBlamePermissions(t *testing.T) {
	err := SDKError(sdkRateLimitErr())

	assert.NotContains(t, err.Error(), "permission denied")
}

// What the message must still say, so dropping the suffix does not cost the operator anything.
func TestRateLimitErrorMessageKeepsWhatIsActionable(t *testing.T) {
	err := SDKError(sdkRateLimitErr())

	assert.Contains(t, err.Error(), "rate limit")
	assert.Contains(t, err.Error(), "32s")
	assert.Contains(t, err.Error(), "GET")
	assert.Contains(t, err.Error(), "https://example/api")
}

// The cause stays reachable through Unwrap: classification runs on errors.As, not on text, and
// nothing downstream should change because the message got shorter.
func TestRateLimitErrorStillUnwrapsToTheSdkError(t *testing.T) {
	cause := errors.New("permission denied when making request to external service")
	err := SDKError(makeSDKErr(http.StatusTooManyRequests, http.Header{}).WithErrors(cause))

	assert.True(t, IsRateLimitExceeded(err))
	assert.Equal(t, cause.Error(), errors.Unwrap(err).Error())
}
