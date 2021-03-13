#! /bin/bash
set -euo pipefail

sudo -E \
     PATH=$PATH \
     FC_TEST_TAP=fc-$BUILDKITE_STEP_KEY-tap$BUILDKITE_BUILD_NUMBER \
     "$@"
