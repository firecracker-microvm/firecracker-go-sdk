TODO
===

Features
---

There's currently no support for specifying rate limits for network or block device access.

Better integration with CNI could be nice. Currently, the network namespace should be created ahead of time by and, and the CNI plugins should be run within it to configure the network. Then the firecracker process (`cmd/firectl/firectl` or whatever process is calling the library) can join that namespace and attach to the appropriate TAP device. At least some of this workflow should be integrated into the library.

Integration with the `jailer`

The tests rely on a bunch of hardcoded information and priviledged access. This should be fixed.
