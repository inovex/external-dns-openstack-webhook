import os
import sys
import time

import openstack
from otcextensions import sdk


def main() -> int:
    cloud = os.environ.get("OS_CLOUD", "")
    zone_id = os.environ.get("ZONE_ID", "")
    if not cloud or not zone_id:
        return 0

    conn = openstack.connect(cloud=cloud)
    sdk.register_otc_extensions(conn)

    conn.dns.delete_zone(zone_id, ignore_missing=True)

    for _ in range(60):
        zone = conn.dns.find_zone(zone_id, ignore_missing=True)
        if zone is None:
            return 0
        time.sleep(2)

    raise RuntimeError(f"zone {zone_id} was not deleted in time")


if __name__ == "__main__":
    sys.exit(main())
