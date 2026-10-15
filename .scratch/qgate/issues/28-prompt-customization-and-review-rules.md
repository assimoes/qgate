# 28: Prompt customization and review rules

**What to build:** Adopting teams teach reviewers their conventions and enforce house rules whose violations are first-class rules.

**Blocked by:** 25 (First reviewer end to end)

**Status:** ready-for-agent

- [ ] `prompt.extend` files append in order; a team file can replace the role prompt; the core preamble and output contract cannot be removed
- [ ] Team files are templates with `.Language`, `.Stage`, `.ProjectName`, `.Categories`; a missing variable exits 2
- [ ] Matching review rules (path glob and target agents) are rendered as a numbered list with IDs
- [ ] A finding citing a valid `review_rule_id` gets rule ID `review:<id>` and the review rule's severity and category; an unknown or non-matching ID makes it `invalid`
- [ ] Review-rule findings can be suppressed and gated like any other rule
