# Tasks: Device Partner-to-Tenant Resolution

## Documentation
- [X] Add tenant-resolution OpenSpec docs for partnerId-based device resolution.

## Discovery
- [X] Identify current default-tenant resolution points in xconfwebconfig device-facing request paths.

## Implementation
- [X] Implement a centralized partnerId -> tenantId resolver reusable by xconfwebconfig where practical.
- [X] Add externally configurable partner-to-tenant mapping support.
- [ ] Wire resolver into device request paths in xconfwebconfig.
- [ ] Complete remaining xconfwebconfig API migration to partner-based tenant mapping (/info/refreshAll).
- [ ] Complete remaining xconfwebconfig API migration to partner-based tenant mapping (/info/refresh/{tableName}).
- [ ] Complete remaining xconfwebconfig API migration to partner-based tenant mapping (/info/statistics).
- [ ] Complete remaining xconfwebconfig API migration to partner-based tenant mapping (/estbfirmware/changelogs).
- [ ] Complete remaining xconfwebconfig API migration to partner-based tenant mapping (/estbfirmware/lastlog).

## Required Behavior Validation
- [X] Ensure missing/blank partnerId resolves to configured default tenant.
- [X] Ensure mapped partner aliases resolve to configured tenant.
- [X] Ensure unmapped partnerId resolves to partnerId itself.
- [X] Ensure tenant resolution is deterministic and never returns blank.
- [X] Ensure matching behavior is case-insensitive (preferred) or documented if constrained by existing conventions.

## Testing
- [X] Add unit tests for missing, mapped, unmapped, and case-insensitive partnerId behavior.
- [ ] Add integration/handler tests in xconfwebconfig where feasible.
- [X] Verify legacy device compatibility (no partnerId -> default tenant fallback preserved).

## Out of Scope Guardrails
- [X] Confirm no xconfadmin SAT/Xerxes behavior changes are introduced.
- [X] Confirm no authN/authZ behavior is introduced by this change.
