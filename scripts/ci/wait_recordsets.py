import os
import sys
import time

import openstack
from otcextensions import sdk


def require(name: str) -> str:
    value = os.environ.get(name, "")
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


def main() -> int:
    cloud = require("OS_CLOUD")
    zone_id = require("ZONE_ID")

    conn = openstack.connect(cloud=cloud)
    sdk.register_otc_extensions(conn)

    for _ in range(60):
        pending = 0
        for recordset in conn.dns.recordsets(zone_id):
            status = str(getattr(recordset, "status", "")).upper()
            if status.startswith("PENDING"):
                pending += 1

        if pending == 0:
            return 0

        time.sleep(2)

    raise RuntimeError("recordsets are still pending")


if __name__ == "__main__":
    sys.exit(main())
