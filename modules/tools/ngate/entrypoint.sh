#!/bin/bash
# /var/opt/cprocsp/tmp is required by certmgr for lock files —
# not created by any package postinst, so we must ensure it exists.
mkdir -p /var/opt/cprocsp/tmp

if [ ! -f /var/opt/cprocsp/.certs-initialized ]; then
    /var/lib/dpkg/info/lsb-cprocsp-ca-certs.postinst configure 2>/dev/null || true
    mkdir -p /var/opt/cprocsp
    touch /var/opt/cprocsp/.certs-initialized
fi

exec "$@"
