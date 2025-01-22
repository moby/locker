Generic Locker
==============

This project is a fork of [moby/locker](https://github.com/moby/locker), 
enhanced with generics support. The API has been adjusted to work around the
lack of support for type parameters in Go methods.


locker provides a mechanism for creating finer-grained locking to help
free up more global locks to handle other tasks.

The implementation looks close to a sync.Mutex, however, the user must provide a
reference to use to refer to the underlying lock when locking and unlocking,
and unlock may generate an error.

If a lock with a given name does not exist when `Lock` is called, one is
created.
Lock references are automatically cleaned up on `Unlock` if nothing else is
waiting for the lock.


## Usage

```go
package important

import (
	"sync"
	"time"

	"github.com/glkz/locker"
)

type important struct {
	locks *locker.Locker[string]
	data  map[string]any
	mu    sync.Mutex
}

func (i *important) Get(name string) any {
	locker.Lock(i.locks, name)
	defer locker.Unlock(i.locks, name)
	return i.data[name]
}

func (i *important) Create(name string, data any) {
	locker.Lock(i.locks, name)
	defer locker.Unlock(i.locks, name)

	i.createImportant(data)

	i.mu.Lock()
	i.data[name] = data
	i.mu.Unlock()
}

func (i *important) createImportant(data any) {
	time.Sleep(10 * time.Second)
}
```

For functions dealing with a given name, always lock at the beginning of the
function (or before doing anything with the underlying state), this ensures any
other function that is dealing with the same name will block.

When needing to modify the underlying data, use the global lock to ensure nothing
else is modifying it at the same time.
Since name lock is already in place, no reads will occur while the modification
is being performed.

