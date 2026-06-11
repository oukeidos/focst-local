package cleanup

import (
	"fmt"
	"sync"
)

var (
	mu    sync.Mutex
	hooks []func() error
)

// Register adds a cleanup hook executed in LIFO order.
func Register(hook func() error) {
	if hook == nil {
		return
	}
	mu.Lock()
	hooks = append(hooks, hook)
	mu.Unlock()
}

// RunAll executes all registered hooks and returns a combined error if any fail.
func RunAll() error {
	mu.Lock()
	local := hooks
	hooks = nil
	mu.Unlock()

	var errs []error
	for i := len(local) - 1; i >= 0; i-- {
		if err := local[i](); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("cleanup failed: %v", errs)
}
