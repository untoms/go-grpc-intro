package main

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// Option custom optional preferences
// option available here is only for option that
// we value as a non mandatory option to make gRPC client resilience
// set this options using WithCustomOption on dialOpts
type Option struct {
	grpc.EmptyDialOption

	// Secure: pass this option true if you want to use WithTransportCredentials dialOption
	// false value will add WithInsecure in dialOptions
	// default: false
	Secure bool

	// MaxBackoff upper bound on backoff
	// when there's connection broken, grpc will automatically doing reDial using exponential backoff
	// longer it broken, will make it more longer to come back
	// this options to set maximum delay time interval to retry
	// default: 10 seconds
	MaxBackoff time.Duration

	// FailFast this option will make RPC call error immediately when there's connection issue
	// default: false
	FailFast bool
}

// defaultOption use default preferences
// Recommended to use this DefaultOption
var defaultOption = Option{
	Secure:     false,
	MaxBackoff: 10 * time.Second,
	FailFast:   false,
}

// WithCustomOption custom option to directly specify optional preferences
func WithCustomOption(o Option) grpc.DialOption {
	return o
}

// waitForReadyInterceptor will enforce all RPC call to use WaitForReady call options
// this option will avoid failfast when there's broken connection in very short period of time eg. Hard Restart
// because of this option can causing process stuck in waiting,
// it will enforce to set context deadline 1 seconds if not there before.
func waitForReadyInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
	}

	opts = append(opts, grpc.WaitForReady(true))

	return invoker(ctx, method, req, reply, cc, opts...)
}
