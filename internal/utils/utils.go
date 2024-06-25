package utils

import (
	"github.com/go-logr/logr"
)

type ProviderStatus interface {
	ToStatusMap() (map[string]string, error)
}

type AssertLogger struct {
	logr.Logger
}

func (t AssertLogger) Errorf(msg string, args ...interface{}) {
	t.Error(nil, msg, args...)
}

type ProviderContextKey struct{}

type ProviderError struct {
	Message string
}

func (p ProviderError) Error() string {
	return p.Message
}

// type ExternalProviderRateLimiter struct {
// 	rl workqueue.RateLimiter
// }

// func NewRateLimiter() *ExternalProviderRateLimiter {
// 	return &ExternalProviderRateLimiter{
// 		rl: workqueue.NewItemExponentialFailureRateLimiter(time.Minute, 10*time.Minute),
// 	}
// }

// func (erl *ExternalProviderRateLimiter) When(item interface{}) time.Duration {
// 	return time.Minute
// }

// func (erl *ExternalProviderRateLimiter) Forget(item interface{}) {
// }

// // RateLimiter is an identical interface of client-go workqueue RateLimiter.
// type RateLimiter interface {
// 	// When gets an item and gets to decide how long that item should wait
// 	When(item interface{}) time.Duration
// 	// Forget indicates that an item is finished being retried.  Doesn't matter whether its for perm failing
// 	// or for success, we'll stop tracking it
// 	Forget(item interface{})
// 	// NumRequeues returns back how many failures the item has had
// 	NumRequeues(item interface{}) int
// }
