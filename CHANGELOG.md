# 0.16.0

* Fixes bug where context was not being used at all during startVM (#86)
* Updates the jailer's socket path to point to the unix socket in the jailer's workspace (#86)
* Fixes bug where default socketpath would always be used when not using jailer (#84).

# 0.15.1

* Add the machine.Shutdown() method, enabling access to the SendCtrlAltDel API
  added in Firecracker 0.15.0

# 0.15.0

* Initial release
