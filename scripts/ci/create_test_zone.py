import os
import sys
import time

import openstack
from otcextensions import sdk
from otcextensions.sdk.dns.v2.zone import Router


def require(name: str) -> str:
    value = os.environ.get(name, "")
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


def getenv_default(name: str, default: str) -> str:
    value = os.environ.get(name, "")
    if value:
        return value
    return default


def main() -> int:
    cloud = require("OS_CLOUD")
    zone_type = require("MATRIX_ZONE_TYPE")
    zone_email = getenv_default("OS_ZONE_EMAIL", "ext-public@t-cloud.ext")
    public_suffix = getenv_default("OS_PUBLIC_ZONE_SUFFIX", "ext-public")
    private_suffix = getenv_default("OS_PRIVATE_ZONE_SUFFIX", "ext-private")
    router_id = os.environ.get("OS_PRIVATE_ROUTER_ID", "")
    router_region = getenv_default("OS_PRIVATE_ROUTER_REGION", "eu-de")
    github_output = require("GITHUB_OUTPUT")
    run_id = require("GITHUB_RUN_ID")
    run_attempt = require("GITHUB_RUN_ATTEMPT")

    zone_suffix = private_suffix if zone_type == "private" else public_suffix
    if not zone_suffix:
        raise RuntimeError(f"missing zone suffix for {zone_type} zone")
    if zone_type == "private" and not router_id:
        raise RuntimeError("OS_PRIVATE_ROUTER_ID is required for private zone creation")

    zone_name = f"ci-{run_id}-{run_attempt}-{zone_type}.{zone_suffix.lstrip('.')}".rstrip(".") + "."

    conn = openstack.connect(cloud=cloud)
    sdk.register_otc_extensions(conn)

    attrs = {
        "name": zone_name,
        "zone_type": zone_type,
        "email": zone_email,
        "ttl": 300,
    }
    if zone_type == "private":
        attrs["router"] = Router(router_id=router_id, router_region=router_region)

    zone = conn.dns.create_zone(**attrs)

    for _ in range(60):
        current = conn.dns.get_zone(zone.id)
        status = str(getattr(current, "status", "")).upper()
        if status == "ACTIVE":
            with open(github_output, "a", encoding="utf-8") as fh:
                fh.write(f"zone_id={current.id}\n")
                fh.write(f"zone_name={current.name}\n")
            return 0
        if status == "ERROR":
            raise RuntimeError(f"zone creation failed for {current.id}")
        time.sleep(2)

    raise RuntimeError(f"zone {zone.id} did not become ACTIVE in time")


if __name__ == "__main__":
    sys.exit(main())
