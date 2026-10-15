# 16: GitLab comments

**What to build:** GitLab merge requests get the same summary and inline experience as GitHub.

**Blocked by:** 15 (GitHub inline comments)

**Status:** ready-for-agent

- [ ] Summary note upserted by marker per stage
- [ ] Inline discussions with position objects and `suggestion:-0+0` blocks, marker dedup and the same caps and diff rules as GitHub
- [ ] Self-managed GitLab works via `reporting.pr.api_url`; platform auto-detected or set by `reporting.pr.platform`
- [ ] Tested with go-vcr recorded HTTP
