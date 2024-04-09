// This is a fake go.mod
// Older versions of go have different go get/go install semantics. In particular:
// 1. `go get` with `GO111MODULES=off` will retrieve and and install a package, but you can't specify a version
// 2. `go get` with `GO111MODULES=on` will retrive a packge at a specific version, but messes with the go.mod. The package can then be installed with `go install`

// We don't actually want binary dependencies to modify the go.mod but since we want to pin versions, we create this unused go.mod
// that `go get` can mess with without affecting our main codebase.
module hack

go 1.11

require (
	github.com/awslabs/tc-redirect-tap v0.0.0-20240408144842-496fddc89db6 // indirect
	github.com/containernetworking/plugins v1.1.1 // indirect
	github.com/kunalkushwaha/ltag v0.2.3 // indirect
)
