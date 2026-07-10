# Tasks: Device Partner-to-Tenant Resolution

## Documentation
- [ ] Add tenant-resolution OpenSpec docs for partnerId-based device resolution.

## Discovery
- [ ] Identify current default-tenant resolution points in xconfwebconfig device-facing request paths.
- [ ] Identify current default-tenant resolution points in xconfds device-facing request paths.

## Implementation
- [ ] Implement a centralized partnerId -> tenantId resolver reusable by xconfwebconfig and xconfds where practical.
- [ ] Add externally configurable partner-to-tenant mapping support.
- [ ] Wire resolver into device request paths in xconfwebconfig.
- [ ] Wire resolver into device request paths in xconfds.

## Required Behavior Validation
- [ ] Ensure missing/blank partnerId resolves to configured default tenant.
- [ ] Ensure mapped partner aliases resolve to configured tenant.
- [ ] Ensure unmapped partnerId resolves to partnerId itself.
- [ ] Ensure tenant resolution is deterministic and never returns blank.
- [ ] Ensure matching behavior is case-insensitive (preferred) or documented if constrained by existing conventions.

## Testing
- [ ] Add unit tests for missing, mapped, unmapped, and case-insensitive partnerId behavior.
- [ ] Add integration/handler tests in xconfwebconfig where feasible.
- [ ] Add integration/handler tests in xconfds where feasible.
- [ ] Verify legacy device compatibility (no partnerId -> default tenant fallback preserved).

## Out of Scope Guardrails
- [ ] Confirm no xconfadmin SAT/Xerxes behavior changes are introduced.
- [ ] Confirm no authN/authZ behavior is introduced by this change.
