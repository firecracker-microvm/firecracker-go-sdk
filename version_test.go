package firecracker

import (
	"io/ioutil"
	"regexp"
	"testing"
)

func TestVersionAndChangelogSync(t *testing.T) {
	const changelogFilename = "CHANGELOG.md"
	b, err := ioutil.ReadFile(changelogFilename)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", changelogFilename, err)
	}

	clVersion := getVersion(b)
	if clVersion != Version {
		t.Errorf("CHANGELOG version does not match Version: %v != %v", clVersion, Version)
	}
}

var versionRegex = regexp.MustCompile("# [0-9]+.[0-9]+.[0-9]+")

func getVersion(contents []byte) string {
	version := string(versionRegex.Find(contents))
	if len(version) == 0 {
		return ""
	}

	// strip off the "# "
	return version[2:]
}
