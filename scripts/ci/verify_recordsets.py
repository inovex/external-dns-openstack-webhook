import os
import sys

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

    a_count = 0
    txt_count = 0

    for recordset in conn.dns.recordsets(zone_id):
        record_type = str(getattr(recordset, "type", "")).upper()
        if record_type == "A":
            a_count += 1
        elif record_type == "TXT":
            txt_count += 1

    if a_count < 10:
        raise RuntimeError(f"expected at least 10 A recordsets, got {a_count}")

    if txt_count < 10:
        raise RuntimeError(f"expected at least 10 TXT recordsets, got {txt_count}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
