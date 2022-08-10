# Snapshotting

Snapshotting is currently supported in the Firecracker Go SDK using Firecracker v1.0.0's API.

Due to [known issues and limitations](https://github.com/firecracker-microvm/firecracker/blob/firecracker-v1.0/docs/snapshotting/snapshot-support.md#known-issues-and-limitations), it is currently not recommended to use snapshots in production.

Snapshots created in this version only save the following:
- guest memory
- emulated hardware state (both KVM & Firecracker emulated hardware)

Each of the above are saved in its own separate file. Anything else is up to the user to restore (tap devices, drives, etc.).

In particular, drives must be in the same location as they were when loading the snapshot. Otherwise, the API call will fail. Changing said drive file can lead to some unexpected behaviors, so it is recommended to make minimal changes to the drive.

Snapshots can only be loaded upon device startup. Upon loading the snapshot, the emulated hardware state is restored, and normal VM activites can resume right where they left off.

Read more in-depth documentation on Firecracker's snapshotting tool [here](https://github.com/firecracker-microvm/firecracker/blob/firecracker-v1.0/docs/snapshotting/snapshot-support.md).

## Using Snapshots via Firecracker Go SDK

Snapshots can be created via a machine object's `CreateSnapshot()` function. The call will make the snapshot files at the specified paths, with the memory saved to `memPath`, and the machine state saved to `snapPath`. The VM must be paused beforehand.

```
import (
	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
)

...

ctx := context.Background()
cfg := sdk.Config{

    ...

}

m, _ := sdk.NewMachine(ctx, cfg)
m.Start(ctx)
m.PauseVM(ctx)
m.CreateSnapshot(ctx, memPath, snapPath)
```

The snapshot can be loaded at any later time at creation of a machine via the machine's `NewMachine()` function, using `WithSnapshot()` as an option. Upon starting, the VM loads the snapshot and must then be resumed before attempting to use it.

```
ctx := context.Background()
cfg := sdk.Config{

    ...

}
m, _ := sdk.NewMachine(ctx, cfg, sdk.WithSnapshot(memPath, snapPath))

m.Start(ctx)
m.ResumeVM(ctx)
```

Check out [examples/cmd/snapshotting](../examples/cmd/snapshotting) for a quick example that can be run on your machine.