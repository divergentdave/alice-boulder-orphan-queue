#!/bin/bash
set -e
trap 'echo $0: error on line ${LINENO}' ERR

TOPSRCDIR="$(realpath "$(dirname "$0")")"
export ALICE_HOME="$TOPSRCDIR/alice"
PATH="$PATH:$TOPSRCDIR/alice/bin"
source "$TOPSRCDIR/.venv/bin/activate"

rm -rf checker_logs

# Build
(cd harness/verifier; go build .)

# Check possible execution traces with the verifier.
alice-check --traces_dir=traces_dir \
    --log_dir=checker_logs \
    --checker=./harness/verifier/verifier
