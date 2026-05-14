package activity_test

import (
	"go.temporal.io/sdk/activity"
)

// registerOpts is shared sugar so test files don't repeat the import alias.
func registerOpts(name string) activity.RegisterOptions {
	return activity.RegisterOptions{Name: name}
}
