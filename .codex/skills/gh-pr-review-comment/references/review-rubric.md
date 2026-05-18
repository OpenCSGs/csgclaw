# PR Review Rubric

Use this rubric when preparing review findings and deciding which findings are worth publishing as PR comments.

## Severity Calibration

### Critical

Use for issues that likely require immediate correction before merge:

- Security vulnerabilities or permission bypass.
- Data corruption or irreversible data loss.
- Clearly broken logic on a main execution path.
- Crashes or deadlocks that are very likely in normal use.

Default publication: `yes`

### High

Use for meaningful defects or regressions that should usually block merge:

- Incorrect behavior for common inputs.
- Contract or compatibility breaks without an intentional migration path.
- Missing validation that plausibly causes production failures.
- Concurrency, lifecycle, or state bugs with real operational impact.

Default publication: `yes`

### Medium

Use for genuine but narrower issues:

- Edge-case correctness problems.
- Missing regression coverage for risky changed behavior.
- Non-trivial maintainability issues that make future mistakes likely.
- Performance problems that are credible but not obviously severe.

Default publication: `ask`

### Low

Use for optional improvements:

- Naming, readability, or structure suggestions.
- Minor refactors with no clear defect.
- Small test quality improvements.

Default publication: `no`

## Comment Quality Bar

Publish only comments that clear all of these checks:

- The problem is grounded in the current diff or nearby touched code.
- The comment names the impact, not just the code smell.
- The suggestion is concrete enough for the author to act on.
- The tone is professional and non-speculative.

Prefer not to publish when:

- The issue is purely stylistic and local conventions are unclear.
- The concern depends on large unseen context.
- The finding is duplicative of a stronger nearby comment.
- The comment can be folded into the summary without losing value.

## Wording Pattern

Use a compact structure:

1. State the issue.
2. State the risk or impact.
3. Suggest the fix or the validation needed.

Example shape:

`This changes <behavior>. In <case>, that means <risk>. Consider <fix or test>.`

## Selection Rules

If the user does not specify what to post:

- Post `critical` and `high`.
- Hold `medium` for confirmation.
- Keep `low` in the summary only.

When the execution path is `gh`:

- Prefer inline comments posted through `gh api` over a generic review body.
- Use a top-level `gh pr review --comment` body only for summary-only feedback or when inline anchoring is not possible.
- If line anchoring is uncertain, do not guess. Refresh PR metadata and rebuild the posting plan first.

If there are no findings worth publishing:

- Say that explicitly.
- Provide a concise summary of what was checked.
- Do not invent low-value comments.
