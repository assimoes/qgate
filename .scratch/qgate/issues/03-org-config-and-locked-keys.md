# 03: Org config and locked keys

**What to build:** A platform team publishes an org config that every repo inherits and can lock keys no later layer may change.

**Blocked by:** 02 (Repo config and profiles)

**Status:** resolved

- [x] `--org-config` accepts a path or HTTPS URL and is layered between builtin defaults and the repo config
- [x] A URL org config pinned with `org_config_sha256` loads on match and exits 2 on mismatch
- [x] Changing a locked key from the repo config, a profile or `--set` exits 2 naming the key and the layer
- [x] `qgate validate` shows the effective value and which layer set it for locked keys
- [x] List merge rules between org and repo config are honored: inputs merge by name, review rules by id, suppressions are appended, agents deep-merge (implemented in 02; first observable with two layers that both carry lists)
