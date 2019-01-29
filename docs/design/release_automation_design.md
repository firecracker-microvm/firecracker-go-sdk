## Release Automation Process Design

### Abstract

The release automation process is an automated system that will handle Github
releases. What this entails is tagging a release, ensure that the release blurb
contains all the changes that are to go out, and potentially handle release
artifacts.

### Specification

The release automation process will be triggered by a periodic AWS Lambda
(Lambda) job that will signify whether or not regeneration of the SDK needs to
occur and/or tag a Github release.

Regeneration of the SDK will be determined by diffing the swagger.yaml files in
the SDK and Firecracker. If there is a difference in the swagger files, this
will regenerate the SDK and also tag a new release. In the event that there are
no differences, the SDK will check some sort of notifier that may trigger a
newly tagged release. An example notifier could be a certain change in the
CHANGELOG.md file which would flag for a release in Amazon DynamoDB (DynamoDB).
By storing a release flag in a database allows for multiple different ways for
triggering a release and allows for swapping the type of release notifier.

The newly generated SDK will need to be tested before deployment or before
tagging a release, which will use the GitLab test runner to do the testing.
This will require some modifications on how code is deployed, meaning a
separate branch.  Once that branch has been committed to the release process
would need to create a PR and poll for success of the GitLab test runner.

### Deployment

CDK and AWS CloudFormation templates are an option, but this writing this stack
will be short work in the AWS SDK for Go. The release process stack can be
deployed to a new stack by simply running its make command with the given set
of AWS credentials for the new stack. This will setup the proper DynamoDB
tables, Lambda functions, IAM roles, and any other resources needed.

### Manual Release Process

Ensure that no PRs, commits, or other releases are in the process during the
release process. It should be communicated to the team that a release is
occurring to prevent bad releases.

Once it has been communicated that the release is about to being, the releaser
must put on the release hat. You are now ready to start the release.

1. Install release dependency if it is not already installed. `go get github.com/aktau/github-release`
2. Clone the upstream repo. `git clone git@github.com:firecracker-microvm/firecracker-go-sdk.git`
3. cd into the clones repository.
4. Change to the `staging` branch to stage changes. `git checkout staging`
5. Edit the CHANGELOG.md with a description of the changes to go out.
6. Modify the version.go file to contain the new version.
7. Run the unit tests, `make test`, and ensure they pass.
8. Prep the release by creating a github tag, `git tag v1.0.0 && git push --tags`
9. Create the release.
```
GITHUB_TOKEN=token github-release release \
	--user username \
	--repo reponame \
	--tag v1.0.0 \
	--name "Release v1.0.0" \
	--description "CHANGELOG.md contents here" \
```
10. Ensure that the release has gone out and that the release in GitHub looks
    like the release that was just created.

### Questions

Can we generalize this outside of just the SDK like firectl? 
We should be able to. However, figuring out how to deal with artifacts is
questionable. I imagine no single Lambda trigger would be used for the SDK,
firectl, and containderd. I imagine, we’d want to use a separate trigger for
all three. The benefit with that it will allow for customization of how
binaries are handled.

How do we want to utilize milestones?
TBD
