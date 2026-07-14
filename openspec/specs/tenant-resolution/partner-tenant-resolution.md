# Spec: Partner-to-Tenant Resolution for Device-Facing Requests

## Status
Proposed

## Context
This specification defines tenantId resolution for device-facing requests handled by xconfwebconfig.

xconfadmin tenant behavior remains separate and auth-based; this spec does not apply to xconfadmin SAT/Xerxes admin APIs.

## Definitions
- partnerId: Device-provided partner identifier in request context.
- tenantId: Effective tenant identifier used for data access/rule evaluation.
- default tenant: Configured fallback tenant used when tenant cannot otherwise be determined.
- partner-to-tenant mapping: Configurable mapping that resolves a partnerId to the tenantId used for data access and rule evaluation. Multiple partnerIds may resolve to the same tenantId.

## Requirements
1. Device-facing tenant resolution MUST use partnerId as the primary input.
2. Device-facing services MUST NOT require tenantId from devices.
3. SAT MUST NOT be part of device-facing tenant resolution for this spec.
4. If partnerId is missing, blank, unknown, or noaccount (case-insensitive), resolver MUST return configured default tenant.
5. If partnerId is present, resolver MUST check configured partner-to-tenant mapping.
6. Mapping lookup SHOULD be case-insensitive; if implementation constraints require case-sensitive behavior, this MUST be explicitly documented and tested.
7. If partnerId matches any configured alias, resolver MUST return the mapped canonical tenant.
8. If partnerId does not match any configured alias, resolver MUST return partnerId as tenantId.
9. Resolver MUST be deterministic for the same input/configuration.
10. Resolver MUST never return blank tenantId.
11. Mapping configuration MUST be external/config-driven, not hardcoded.
12. The behavior defined by this spec MUST apply consistently across xconfwebconfig device-facing paths.

## Conceptual Mapping Model
Examples:
- tenantA -> [partner1, partner2, partner3, partner4]
- tenantB -> [partner5, partner6]
- tenantC -> [partner7, partner8]

Exact config key names/syntax are implementation details.

## Behavior Examples
- partnerId missing -> default tenant
- partnerId=unknown -> default tenant
- partnerId=noaccount -> default tenant
- partnerId=partner1 -> tenantA
- partnerId=partner2 -> tenantA
- partnerId=partner3 -> tenantA
- partnerId=partner4 -> tenantA
- partnerId=partner5 -> tenantB
- partnerId=partner6 -> tenantB
- partnerId=partner7 -> tenantC
- partnerId=partner8 -> tenantC
- partnerId=someNewPartner with no mapping -> someNewPartner

## Compatibility and Non-Goals
- No requirement for devices to send tenantId.
- No SAT/Auth changes.
- No xconfadmin behavior changes.
- This spec resolves tenant context only and does not imply partner authorization.
