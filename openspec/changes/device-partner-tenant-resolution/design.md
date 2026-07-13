# Design: Device Partner-to-Tenant Resolution

## Scope
Applies to device-facing request processing in:
- xconfwebconfig

Out of scope:
- xconfadmin SAT/Xerxes admin APIs
- Authentication and authorization policy

## Design Goals
- Keep device-facing tenant resolution deterministic.
- Preserve legacy behavior for devices missing partnerId.
- Keep mapping externally configurable.
- Avoid duplicating tenant-resolution logic across xconfwebconfig device paths.

## Resolution Flow
1. Read partnerId from device request context.
2. Normalize partnerId for matching (preferred: case-insensitive, trimmed).
3. If partnerId is missing/blank, return configured default tenant.
4. If partnerId is unknown or noaccount (case-insensitive), return configured default tenant.
5. If partnerId matches a configured alias, return mapped tenant.
6. If partnerId is present but not mapped, return partnerId itself.
7. Ensure final tenantId is never blank.

## Decision Table
| Input partnerId | Mapping match | Output tenantId |
|---|---|---|
| missing/blank | n/a | default tenant |
| unknown | n/a | default tenant |
| noaccount | n/a | default tenant |
| partner1 | tenantA -> [partner1, partner2, partner3, partner4] | tenantA |
| partner2 | tenantA -> [partner1, partner2, partner3, partner4] | tenantA |
| partner3 | tenantA -> [partner1, partner2, partner3, partner4] | tenantA |
| partner4 | tenantA -> [partner1, partner2, partner3, partner4] | tenantA |
| someNewPartner | no | someNewPartner |

## Config-Driven Mapping
Mapping must be externally configurable (not hardcoded). Conceptual model:
- tenantA -> [partner1, partner2, partner3, partner4]
- tenantB -> [partner5, partner6]
- tenantC -> [partner7, partner8]

Exact config syntax/key naming is implementation detail and should follow existing config conventions if present.

## Fallback Behavior
- Missing/blank partnerId uses existing default-tenant behavior.
- partnerId values unknown/noaccount (case-insensitive) use existing default-tenant behavior.
- Unmapped partnerId resolves to itself.
- Resolution must never return blank.

## Compatibility
- Legacy devices not sending partnerId continue using default tenant.
- Devices are not required to send tenantId.
- Existing default-tenant fallback remains intact.
- No SAT/Auth changes.

## Reuse and Centralization
Where practical, implement a shared resolver helper used by xconfwebconfig device paths to reduce drift and ensure consistent behavior.
