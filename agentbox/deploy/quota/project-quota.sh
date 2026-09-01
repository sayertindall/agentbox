#!/bin/sh
# Host-approved project-quota backend. The Go quota.Memory backend is the unit
# test stand-in. This script is the deployment hook the VPS audit must bind to a
# real filesystem quota mechanism before a real transfer.
set -eu
echo "project-quota backend is not provisioned on this host" >&2
exit 1
