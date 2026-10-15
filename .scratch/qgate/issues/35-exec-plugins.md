# 35: Exec plugins

**What to build:** Adopting teams integrate any executable as an input through a versioned JSON protocol.

**Blocked by:** 33 (Gate: Phase 4 acceptance)

**Status:** ready-for-agent

- [ ] qgate writes one schema-valid request to stdin and reads one response from stdout; stderr goes to debug logs
- [ ] Responses failing schema validation, and non-zero exits, are input failures
- [ ] qgate namespaces plugin rule IDs and fills source, fingerprint and status
- [ ] Request and response JSON Schemas are published
- [ ] A sample Go plugin round-trips through the protocol
