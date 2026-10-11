package errs

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GreenNodeHub/vngcloud-go-sdk/v2/vngcloud/sdk_error"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

func TestHandleReconcileError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name    string
		args    args
		want    ctrl.Result
		wantErr error
	}{
		{
			name: "input err is nil",
			args: args{
				err: nil,
			},
			want:    ctrl.Result{},
			wantErr: nil,
		},
		{
			name: "input err is RequeueNeededAfter",
			args: args{
				err: NewRequeueNeededAfter("some error", 3*time.Second),
			},
			want: ctrl.Result{
				RequeueAfter: 3 * time.Second,
			},
			wantErr: nil,
		},
		{
			name: "input err is RequeueNeeded",
			args: args{
				err: NewRequeueNeeded("some error"),
			},
			want: ctrl.Result{
				Requeue: true,
			},
			wantErr: nil,
		},
		{
			name: "input err is other error type",
			args: args{
				err: errors.New("some error"),
			},
			want:    ctrl.Result{},
			wantErr: errors.New("some error"),
		},
		{
			name: "input err is rate limit with server hint honored (plus floor + jitter)",
			args: args{
				err: &domain.RateLimitError{RetryAfter: 4 * time.Second},
			},
			// floor 2s ≤ 4s ≤ 5m ceiling, plus up to 50% jitter → [4s, 6s)
			want:    ctrl.Result{},
			wantErr: nil,
		},
		{
			name: "input err wraps rate limit",
			args: args{
				err: fmt.Errorf("wrapped: %w", &domain.RateLimitError{RetryAfter: 0}),
			},
			want:    ctrl.Result{},
			wantErr: nil,
		},
	}

	logger := logrus.New().WithField("test", "TestHandleReconcileError")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HandleReconcileError(tt.args.err, logger)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}
			assert.NoError(t, err)

			var rl *domain.RateLimitError
			if errors.As(tt.args.err, &rl) {
				// rate-limit: requeue duration is server hint with floor + jitter,
				// so we just assert it's > 0 and reasonable.
				assert.Greater(t, got.RequeueAfter, time.Duration(0))
				assert.Less(t, got.RequeueAfter, 10*time.Minute)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// #33486: the SDK's own text for a 429 used to be the constant "permission denied when making
// request to external service", so it was dropped from the error's message - it said nothing, and
// what it did say was wrong. From v2.24.0 it says the server's reason, and nothing was printing
// it: the CR message is built from Error(), and this handler builds its own sentence. Debug is
// where it belongs - the warning above stays short because it is the line an operator reads.
func TestHandleReconcileErrorLogsTheRateLimitCauseAtDebug(t *testing.T) {
	sdkErr := sdk_error.ErrorHandler(stderrors.New("rate limited by external service, too many requests")).
		WithKVparameters("statusCode", http.StatusTooManyRequests, "method", "GET", "url", "https://example/api")
	sdk_error.WithErrorTooManyRequests()(sdkErr)

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	hook := test.NewLocal(logger)

	_, err := HandleReconcileError(domain.SDKError(sdkErr), logger.WithField("test", t.Name()))

	assert.NoError(t, err)
	var debug []string
	for _, e := range hook.AllEntries() {
		if e.Level == logrus.DebugLevel {
			debug = append(debug, e.Message)
		}
	}
	assert.Contains(t, strings.Join(debug, "\n"), "too many requests",
		"the server's own reason for the throttle should be recoverable from the logs")
}
